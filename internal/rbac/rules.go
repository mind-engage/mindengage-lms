package rbac

// Simple default policy. Expand as needed.
var RolePermissions = map[string][]string{
	"student": {
		"exam:view",
		"attempt:create",
		"attempt:save",
		"attempt:submit",
		"attempt:view-own",
		"user:change_password",
	},
	"teacher": {
		"course:delete_own",
		"exam:create",
		"exam:delete_own",
		"exam:view",
		"exam:export",
		"attempt:view-all",
		"attempt:grade",
		"users:bulk_upsert",
		"users:list",
	},
	"admin": {
		"*", // everything
	},
}
