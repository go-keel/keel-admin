package i18n

import (
	"context"

	kerrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// Localed builds a sentinel OnLocaled handler: it looks up the localized message by messageID and passes it to the
// matching typed error_reason constructor (v1.ErrorXxx). A missing message (the current language has no translation,
// only the English source) is not a failure and falls back to English through MustLocalizeMessage; a panic is
// reserved for real faults such as a template that fails to parse. The kernel and the business area both use it.
func Localed(messageID, fallback string, build func(format string, args ...interface{}) *kerrors.Error) func(ctx context.Context, l *i18n.Localizer, err error) error {
	return func(_ context.Context, l *i18n.Localizer, _ error) error {
		return build("%s", MustLocalizeMessage(l, &i18n.Message{ID: messageID, Other: fallback}))
	}
}
