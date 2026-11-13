package data

import (
	"context"

	"log/slog"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

var _ biz.LoginLogger = (*loginLogger)(nil)

type loginLogger struct {
	data *coredata.Data
}

// NewLoginLogger builds the login-log writer.
func NewLoginLogger(data *coredata.Data) biz.LoginLogger {
	return &loginLogger{data: data}
}

func (l *loginLogger) Log(ctx context.Context, e biz.LoginEntry) {
	status := int8(0)
	if e.Success {
		status = 1
	}
	m := sysLoginLog{
		UserID:   e.UserID,
		Username: e.Username,
		Status:   status,
		IP:       e.IP,
		Msg:      e.Msg,
	}
	if err := l.data.DB.WithContext(ctx).Create(&m).Error; err != nil {
		slog.ErrorContext(ctx, "write login log failed", "error", err)
	}
}

func (l *loginLogger) List(ctx context.Context, f biz.LoginFilter, offset, limit int) ([]*biz.LoginRecord, int64, error) {
	q := l.data.DB.WithContext(ctx).Model(&sysLoginLog{})
	if f.UserID > 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.Username != "" {
		// Prefix / substring match: someone checking "did this person fail to log in" often remembers half a name,
		// and the username in a failure record is exactly what was typed (the account may not exist at all).
		q = q.Where("username LIKE ?", containsPattern(f.Username))
	}
	switch f.Status {
	case 1:
		q = q.Where("status = 1")
	case 2:
		q = q.Where("status <> 1")
	}
	if f.IP != "" {
		q = q.Where("ip = ?", f.IP)
	}
	if !f.Start.IsZero() {
		q = q.Where("created_at >= ?", f.Start)
	}
	if !f.End.IsZero() {
		q = q.Where("created_at < ?", f.End)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var ms []sysLoginLog
	if err := q.Order("id desc").Offset(offset).Limit(limit).Find(&ms).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*biz.LoginRecord, 0, len(ms))
	for i := range ms {
		out = append(out, ms[i].toBiz())
	}
	return out, total, nil
}
