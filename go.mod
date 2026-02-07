module github.com/suprsokr/mithril

go 1.22.0

require (
	github.com/go-sql-driver/mysql v1.9.3
	github.com/suprsokr/go-mpq v0.0.0
)

require filippo.io/edwards25519 v1.1.0 // indirect

replace github.com/suprsokr/go-mpq => ../go-mpq
