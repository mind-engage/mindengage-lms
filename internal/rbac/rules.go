package rbac

// Simple default policy. Expand as needed.
var RolePermissions = map[string][]string{
	"student": {
		"exam:view",
		"attempt:create",
		"attempt:save",
		"attempt:submit",
		"attempt:view-own",
	},
	"teacher": {
		"exam:create",
		"exam:view",
		"attempt:view-all",
		"attempt:grade",
	},
	"admin": {
		"*", // everything
	},
}
