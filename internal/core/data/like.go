package data

import "strings"

// likeEscaper escapes the LIKE wildcards and the escape character, so a keyword matches literally. MySQL and
// PostgreSQL both use a backslash as the default LIKE escape character.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// ContainsPattern is the LIKE pattern for "contains keyword", with the keyword taken literally: searching "u_" finds
// "u_mgr", not every name with a "u" followed by any character. Kernel and business repositories both use it.
func ContainsPattern(keyword string) string {
	return "%" + likeEscaper.Replace(keyword) + "%"
}
