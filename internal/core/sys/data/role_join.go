package data

// Whether a role counts has exactly two meanings. The join condition is built only here, never written by hand in a
// query:
//
//   - Held (joinHeldRole): the role counts if it is not deleted, disabled or not. This is the holding relation between
//     user and role, used to show a user's roles, filter users by role, and pin the roles the operator cannot see when
//     editing a user; filtering out disabled roles would silently strip them on save.
//   - Active (joinActiveRole): the role counts only if it is not deleted and enabled. Every query about "what is
//     granted" uses it: permission codes, data scope, resource grants, assignable roles, manageable departments, forced
//     authenticator (disabling a role revokes it, docs/admin/permission-model/function-permissions.md §4.6). A new
//     grant query that uses it cannot forget status.
//
// alias is the name of the joined sys_role in the query (no new alias when it has the same name), and on is the
// column on the other side.

func joinHeldRole(alias, on string) string {
	table := "sys_role"
	if alias != table {
		table += " " + alias
	}
	return "JOIN " + table + " ON " + alias + ".id = " + on + " AND " + alias + ".deleted_at IS NULL"
}

func joinActiveRole(alias, on string) string {
	return joinHeldRole(alias, on) + " AND " + alias + ".status = 1"
}
