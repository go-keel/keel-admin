// Example file: delete the whole file after forking

package service

import (
	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/biz"
	"github.com/go-keel/keel-admin/internal/core/perm"
)

// NoticeBindings declares notice's endpoint -> access requirement map,
// registered into the perm registry via perm.MustRegister — the business
// entry point (contrast core/sys/service.Bindings, registered through
// perm.MustRegisterKernel; see cmd/admin/di.go).
var NoticeBindings = perm.Bindings{
	v1.OperationNoticeServiceListNotice:   {Codes: []perm.Code{biz.PermNoticeView}},
	v1.OperationNoticeServiceCreateNotice: {Codes: []perm.Code{biz.PermNoticeCreate}, Audit: true},
	v1.OperationNoticeServiceDeleteNotice: {Codes: []perm.Code{biz.PermNoticeDelete}, Audit: true},
	// Batch delete and single delete share **one code**: they are the same action, only applied to several rows at
	// once. Two permission codes would create a grant state nobody can explain: "may delete one, may not delete many".
	v1.OperationNoticeServiceBatchDeleteNotice: {Codes: []perm.Code{biz.PermNoticeDelete}, Audit: true},

	// Categories: the option set follows view (form dropdowns and list filters need it; it is data the notice page
	// needs to render, so its threshold must not be higher than the entry's). The management list is the request
	// the "category management" page sends on load, so it shares the code of that page's entry and of
	// create/delete. The list and delete are filtered by category grants. Create/delete change an explicit-assignment option set, so
	// they are audited.
	v1.OperationNoticeServiceListNoticeCategoryOptions: {Codes: []perm.Code{biz.PermNoticeView}},
	v1.OperationNoticeServiceListNoticeCategory:        {Codes: []perm.Code{biz.PermNoticeCategory}},
	v1.OperationNoticeServiceCreateNoticeCategory:      {Codes: []perm.Code{biz.PermNoticeCategory}, Audit: true},
	v1.OperationNoticeServiceDeleteNoticeCategory:      {Codes: []perm.Code{biz.PermNoticeCategory}, Audit: true},
}
