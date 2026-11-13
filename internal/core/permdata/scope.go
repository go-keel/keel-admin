// Package permdata translates a resolved perm.ScopeSpec into GORM row-level
// filters, so every domain's list query applies the same OR-merge /
// deny-all semantics instead of hand-rolling its own scope clause.
package permdata

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/go-keel/keel-admin/internal/core/perm"
)

// Cols names the columns ApplyScope filters on. Tables that don't use the
// default column names pass their own via ApplyScopeWith.
//
// Column names always go through clause.Column so GORM quotes them per dialect; they are never spliced into the SQL
// string. They come from the caller (the derived repo's own tables), and a reserved word (such as MySQL's sensitive)
// spliced in raw is a syntax error. Table-prefixed names (`t.col`) are quoted segment by segment.
//
// PK is the column that ResourceIDs (the explicitly assigned instance set) applies to, "id" by default; a resource's
// own table uses the default. When a parent resource's spec is applied to a child table, set it to the foreign key
// column (for example articles filtered by category grants: ApplyScopeWith(catSpec, Cols{PK: "category_id"})). That is
// the one-line form of a derived resource's "child rows are visible as their parent instance is granted".
type Cols struct{ OwnerID, DeptID, PK string }

var defaultCols = Cols{OwnerID: "owner_user_id", DeptID: "owner_dept_id", PK: "id"}

// ApplyScope applies spec using the default column names
// (owner_user_id/owner_dept_id).
func ApplyScope(spec perm.ScopeSpec) func(*gorm.DB) *gorm.DB {
	return ApplyScopeWith(spec, defaultCols)
}

// ApplyScopeWith translates spec into a row-level filter under GORM's
// Scopes mechanism. Conditions are OR-combined; an ALL spec skips filtering
// entirely; a spec with no active condition denies all rows.
func ApplyScopeWith(spec perm.ScopeSpec, c Cols) func(*gorm.DB) *gorm.DB {
	if c.OwnerID == "" {
		c.OwnerID = defaultCols.OwnerID
	}
	if c.DeptID == "" {
		c.DeptID = defaultCols.DeptID
	}
	if c.PK == "" {
		c.PK = defaultCols.PK
	}
	return func(q *gorm.DB) *gorm.DB {
		if spec.All {
			return q
		}
		cond := q.Session(&gorm.Session{NewDB: true})
		has := false
		if spec.SelfUserID > 0 {
			cond = cond.Or("? = ?", clause.Column{Name: c.OwnerID}, spec.SelfUserID)
			has = true
		}
		if len(spec.DeptIDs) > 0 {
			cond = cond.Or("? IN ?", clause.Column{Name: c.DeptID}, spec.DeptIDs)
			has = true
		}
		if len(spec.ResourceIDs) > 0 {
			cond = cond.Or("? IN ?", clause.Column{Name: c.PK}, spec.ResourceIDs)
			has = true
		}
		if !has {
			return q.Where("1 = 0") // Deny-all fallback.
		}
		return q.Where(cond)
	}
}
