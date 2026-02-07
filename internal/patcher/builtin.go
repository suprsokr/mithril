package patcher

// BuiltinPatches returns patches that mithril can auto-apply.
// These are well-known patches for the WoW 3.3.5a (12340) client.
var BuiltinPatches = map[string]*PatchFile{
	"allow-custom-gluexml": {
		Name:        "allow-custom-gluexml",
		Description: "Disables the client's GlueXML/FrameXML integrity check, allowing modified interface files without a 'corrupt interface files' crash.",
		Patches: []Patch{
			{Address: "0x126", Bytes: []string{"0x23"}},
			{Address: "0x1f41bf", Bytes: []string{"0xeb"}},
			{Address: "0x415a25", Bytes: []string{"0xeb"}},
			{Address: "0x415a3f", Bytes: []string{"0x3"}},
			{Address: "0x415a95", Bytes: []string{"0x3"}},
			{Address: "0x415b46", Bytes: []string{"0xeb"}},
			{Address: "0x415b5f", Bytes: []string{"0xb8", "0x03"}},
			{Address: "0x415b61", Bytes: []string{"0x0", "0x0", "0x0", "0xeb", "0xed"}},
		},
	},
	"large-address-aware": {
		Name:        "large-address-aware",
		Description: "Enables Large Address Aware flag, allowing the client to use more than 2GB of RAM.",
		Patches: []Patch{
			{Address: "0x000126", Bytes: []string{"0x23"}},
		},
	},
}
