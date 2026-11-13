package perm

// Requirement is the access requirement for one gRPC operation, defined in code.
type Requirement struct {
	Codes      []Code // dimension-one permission codes
	AllOf      bool   // true = ALL_OF, false = ANY_OF
	RootOnly   bool   // only root may call
	RequireOTP bool   // operation-level step-up OTP
	Audit      bool   // record to sys_audit_log on success
}
