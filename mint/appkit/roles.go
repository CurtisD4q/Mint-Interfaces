package appkit

// ArgRole is what an option's value is for: a plain value, or a file or
// folder the tool reads or writes. mint-gen decides it from the help text
// (see ClassifyArg in tools/mint-gen) and records it in FlagSpec.Role.
type ArgRole string

const (
	RoleValue      ArgRole = ""            // a plain value (number, name, URL…), or no value at all
	RoleFile       ArgRole = "file"        // a file, but the help does not say whether it is read or written
	RoleDir        ArgRole = "dir"         // a folder, likewise
	RoleInputFile  ArgRole = "input-file"  // a file the tool reads
	RoleInputDir   ArgRole = "input-dir"   // a folder the tool reads from or searches
	RoleOutputFile ArgRole = "output-file" // the file the tool writes its results to
	RoleOutputDir  ArgRole = "output-dir"  // the folder the tool saves files into
	RoleOutputBase ArgRole = "output-base" // a base name the tool adds its own extensions to (nmap -oA)
	RoleLogFile    ArgRole = "log-file"    // a file the tool writes messages or a log to
	RoleConfigFile ArgRole = "config-file" // a settings file the tool reads
	RoleFormat     ArgRole = "format"      // the format of the output (json, xml…)
)

// Roles lists every role, in a stable order.
func Roles() []ArgRole {
	return []ArgRole{RoleValue, RoleFile, RoleDir, RoleInputFile, RoleInputDir, RoleOutputFile,
		RoleOutputDir, RoleOutputBase, RoleLogFile, RoleConfigFile, RoleFormat}
}

// Valid reports whether r is one of the roles above.
func (r ArgRole) Valid() bool {
	for _, k := range Roles() {
		if r == k {
			return true
		}
	}
	return false
}

// Writes reports whether the tool writes to the path given for this role.
func (r ArgRole) Writes() bool {
	return r == RoleOutputFile || r == RoleOutputDir || r == RoleOutputBase || r == RoleLogFile
}
