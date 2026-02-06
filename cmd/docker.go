package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// buildDockerImage builds the mithril-server Docker image from the generated Dockerfile.
// --no-cache is used to ensure freshly-generated Dockerfiles and scripts are
// not masked by stale build-cache layers.
func buildDockerImage(cfg *Config) error {
	return runCmdDir(cfg.MithrilDir, "docker", "build", "--no-cache", "-t", "mithril-server:latest", ".")
}

// writeDockerfile generates the multi-stage Dockerfile that clones and compiles
// TrinityCore 3.3.5 inside an Ubuntu 24.04 image, then produces a slim runtime
// image with MySQL 8.
func writeDockerfile(path string) error {
	return os.WriteFile(path, []byte(dockerfile), 0644)
}

// writeDockerCompose generates the docker-compose.yml that runs the single
// mithril-server container with all necessary volume mounts.
func writeDockerCompose(cfg *Config) error {
	content := fmt.Sprintf(`services:
  server:
    image: mithril-server:latest
    container_name: mithril-server
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "3724:3724"     # authserver
      - "8085:8085"     # worldserver
      - "3306:3306"     # mysql
      - "7878:7878"     # SOAP
    volumes:
      - ./etc:/opt/trinitycore/etc
      - ./data:/opt/trinitycore/data
      - ./log:/opt/trinitycore/log
      - ./mysql:/var/lib/mysql
      - ./tdb:/opt/trinitycore/bin/tdb
      - ./client:/opt/trinitycore/client
    environment:
      MYSQL_ROOT_PASSWORD: %s
      MYSQL_TC_USER: %s
      MYSQL_TC_PASSWORD: %s
    restart: unless-stopped
    stdin_open: true
    tty: true
`, cfg.MySQLRootPassword, cfg.MySQLUser, cfg.MySQLPassword)

	return os.WriteFile(cfg.DockerComposeFile, []byte(content), 0644)
}

// writeContainerScripts writes the bash scripts that run inside the container
// (entrypoint, mysql setup, server launchers, data extractor).
func writeContainerScripts(cfg *Config) error {
	scriptsDir := filepath.Join(cfg.MithrilDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return err
	}

	scripts := map[string]string{
		"entrypoint.sh":      scriptEntrypoint,
		"setup-mysql.sh":     scriptSetupMySQL,
		"run-worldserver.sh": scriptRunWorldserver,
		"run-authserver.sh":  scriptRunAuthserver,
		"extract-data.sh":    scriptExtractData,
	}

	for name, content := range scripts {
		path := filepath.Join(scriptsDir, name)
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Dockerfile
// ---------------------------------------------------------------------------

const dockerfile = `# Mithril TrinityCore 3.3.5 — Ubuntu 24.04 multi-stage build
FROM ubuntu:24.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    git clang cmake make gcc g++ \
    libmysqlclient-dev libssl-dev libbz2-dev \
    libreadline-dev libncurses-dev libboost-all-dev \
    p7zip-full p7zip \
    && update-alternatives --install /usr/bin/cc cc /usr/bin/clang 100 \
    && update-alternatives --install /usr/bin/c++ c++ /usr/bin/clang++ 100 \
    && rm -rf /var/lib/apt/lists/*

RUN git clone -b 3.3.5 --depth 1 \
    https://github.com/TrinityCore/TrinityCore.git /src/TrinityCore

WORKDIR /src/TrinityCore/build
RUN cmake ../ \
    -DCMAKE_INSTALL_PREFIX=/opt/trinitycore \
    -DTOOLS=1 \
    -DWITH_WARNINGS=0 \
    -DCMAKE_C_COMPILER=clang \
    -DCMAKE_CXX_COMPILER=clang++ \
    && make -j $(nproc) \
    && make install

# --- runtime ---
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    mysql-server \
    libmysqlclient21 libssl3 libreadline8t64 \
    libboost-all-dev \
    iproute2 p7zip-full p7zip gosu \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/trinitycore /opt/trinitycore
COPY --from=builder /src/TrinityCore/sql /opt/trinitycore/sql

RUN useradd -m -s /bin/bash trinity \
    && mkdir -p \
        /opt/trinitycore/data \
        /opt/trinitycore/etc \
        /opt/trinitycore/log \
        /opt/trinitycore/bin/tdb \
        /var/run/mysqld \
    && chown -R trinity:trinity /opt/trinitycore \
    && chown -R mysql:mysql /var/run/mysqld

COPY scripts/entrypoint.sh      /usr/local/bin/
COPY scripts/setup-mysql.sh     /usr/local/bin/
COPY scripts/run-worldserver.sh /usr/local/bin/
COPY scripts/run-authserver.sh  /usr/local/bin/
COPY scripts/extract-data.sh   /usr/local/bin/
RUN chmod +x /usr/local/bin/*.sh

WORKDIR /opt/trinitycore
EXPOSE 3724 8085 3306
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
`

// ---------------------------------------------------------------------------
// Container scripts
// ---------------------------------------------------------------------------

const scriptEntrypoint = `#!/bin/bash
set -e

# If a custom command was passed (e.g. extract-data.sh), execute it directly
# instead of running the full server startup flow.
if [ $# -gt 0 ]; then
    exec "$@"
fi

echo "=== Mithril TrinityCore Server ==="

# ---- MySQL ----------------------------------------------------------------
if [ ! -d "/var/lib/mysql/mysql" ]; then
    echo "Initializing MySQL data directory..."
    mysqld --initialize-insecure --user=mysql
fi

echo "Starting MySQL..."
mysqld --user=mysql --datadir=/var/lib/mysql &

echo "Waiting for MySQL..."
for i in $(seq 1 60); do
    mysqladmin ping --silent 2>/dev/null && break
    [ "$i" -eq 60 ] && { echo "ERROR: MySQL did not start."; exit 1; }
    sleep 1
done
echo "MySQL is ready."

/usr/local/bin/setup-mysql.sh

# ---- TDB ------------------------------------------------------------------
if ls /opt/trinitycore/bin/tdb/TDB_full_world_335* 1>/dev/null 2>&1; then
    echo "Copying TDB files to worldserver directory..."
    cp -f /opt/trinitycore/bin/tdb/TDB_full_world_335* /opt/trinitycore/bin/ 2>/dev/null || true
fi

# ---- Servers ---------------------------------------------------------------
echo "Starting authserver..."
/usr/local/bin/run-authserver.sh &
sleep 2

echo "Starting worldserver..."
# Run worldserver in the foreground (without exec) so the container stays
# alive if the server exits unexpectedly.  The trailing wait keeps the
# entrypoint running so Docker doesn't restart the container.
set +e
/usr/local/bin/run-worldserver.sh
WS_EXIT=$?
set -e
if [ $WS_EXIT -ne 0 ]; then
    echo "WARNING: worldserver exited with code $WS_EXIT"
    echo "The container will stay running so you can inspect logs."
    echo "  mithril server logs"
    echo "  docker exec -it mithril-server bash"
fi

# Keep the container alive so the user can inspect / restart manually.
echo "Worldserver stopped. Container staying alive for debugging."
tail -f /dev/null
`

const scriptSetupMySQL = `#!/bin/bash
set -e

MYSQL_ROOT_PASSWORD="${MYSQL_ROOT_PASSWORD:-mithril}"
MYSQL_TC_USER="${MYSQL_TC_USER:-trinity}"
MYSQL_TC_PASSWORD="${MYSQL_TC_PASSWORD:-trinity}"

DB_EXISTS=$(mysql -u root -p"${MYSQL_ROOT_PASSWORD}" -e "SHOW DATABASES LIKE 'world';" 2>/dev/null | grep -c "world" || true)

if [ "$DB_EXISTS" -eq 0 ]; then
    echo "Setting up TrinityCore databases..."

    mysql -u root -e \
        "ALTER USER 'root'@'localhost' IDENTIFIED BY '${MYSQL_ROOT_PASSWORD}';" \
        2>/dev/null || true

    if [ -f /opt/trinitycore/sql/create/create_mysql.sql ]; then
        echo "Running TrinityCore create_mysql.sql..."
        # The upstream SQL uses CREATE DATABASE / CREATE USER without
        # IF NOT EXISTS, which fails when the MySQL data directory is
        # reused across container restarts.
        sed -e 's/CREATE DATABASE/CREATE DATABASE IF NOT EXISTS/gi' \
            -e 's/CREATE USER/CREATE USER IF NOT EXISTS/gi' \
            /opt/trinitycore/sql/create/create_mysql.sql \
            | mysql -u root -p"${MYSQL_ROOT_PASSWORD}"
    else
        echo "Creating databases manually..."
        mysql -u root -p"${MYSQL_ROOT_PASSWORD}" -e "
            CREATE DATABASE IF NOT EXISTS world     DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
            CREATE DATABASE IF NOT EXISTS characters DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
            CREATE DATABASE IF NOT EXISTS auth       DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

            CREATE USER IF NOT EXISTS '${MYSQL_TC_USER}'@'localhost' IDENTIFIED BY '${MYSQL_TC_PASSWORD}';
            GRANT ALL PRIVILEGES ON world.*      TO '${MYSQL_TC_USER}'@'localhost';
            GRANT ALL PRIVILEGES ON characters.* TO '${MYSQL_TC_USER}'@'localhost';
            GRANT ALL PRIVILEGES ON auth.*       TO '${MYSQL_TC_USER}'@'localhost';

            CREATE USER IF NOT EXISTS '${MYSQL_TC_USER}'@'%' IDENTIFIED BY '${MYSQL_TC_PASSWORD}';
            GRANT ALL PRIVILEGES ON world.*      TO '${MYSQL_TC_USER}'@'%';
            GRANT ALL PRIVILEGES ON characters.* TO '${MYSQL_TC_USER}'@'%';
            GRANT ALL PRIVILEGES ON auth.*       TO '${MYSQL_TC_USER}'@'%';

            FLUSH PRIVILEGES;
        "
    fi
    echo "Databases created."
else
    echo "Databases already exist, skipping creation."
fi

# ---- Realmlist address ----------------------------------------------------
# Ensure the realmlist entry points to 127.0.0.1 so the host client can reach
# the worldserver through Docker's port mapping.  We set localSubnetMask to
# 0.0.0.0 so the authserver always returns the address field regardless of
# which subnet the client connects from (e.g. Docker bridge).
echo "Updating realmlist address to 127.0.0.1..."
mysql -u root -p"${MYSQL_ROOT_PASSWORD}" -e "
    UPDATE auth.realmlist
       SET address          = '127.0.0.1',
           localAddress     = '127.0.0.1',
           localSubnetMask  = '0.0.0.0'
     WHERE id = 1;
" 2>/dev/null || echo "WARNING: Could not update realmlist (table may not exist yet — worldserver will create it on first start)."

`

const scriptRunWorldserver = `#!/bin/bash
cd /opt/trinitycore/bin
exec gosu trinity ./worldserver -c /opt/trinitycore/etc/worldserver.conf
`

const scriptRunAuthserver = `#!/bin/bash
cd /opt/trinitycore/bin
exec gosu trinity ./authserver -c /opt/trinitycore/etc/authserver.conf
`

const scriptExtractData = `#!/bin/bash
set -e

CLIENT_DIR="${1:-/opt/trinitycore/client}"
DATA_DIR="/opt/trinitycore/data"
BIN_DIR="/opt/trinitycore/bin"

if [ ! -f "$CLIENT_DIR/Wow.exe" ] && [ ! -f "$CLIENT_DIR/WoW.exe" ]; then
    echo "ERROR: WoW client not found at $CLIENT_DIR"
    exit 1
fi

cd "$CLIENT_DIR"

echo "=== Extracting DBC, maps, and cameras ==="
"$BIN_DIR/mapextractor"
mkdir -p "$DATA_DIR"
cp -r cameras dbc maps "$DATA_DIR/"

echo "=== Extracting vmaps ==="
"$BIN_DIR/vmap4extractor"
mkdir -p vmaps
"$BIN_DIR/vmap4assembler" Buildings vmaps
cp -r vmaps "$DATA_DIR/"

echo "=== Extracting mmaps (this may take a long time) ==="
mkdir -p mmaps
"$BIN_DIR/mmaps_generator"
cp -r mmaps "$DATA_DIR/"

echo "=== Extraction complete ==="
rm -rf cameras dbc maps Buildings vmaps mmaps
`
