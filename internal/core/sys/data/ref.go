package data

import (
	"context"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

var _ biz.RefRepo = (*refRepo)(nil)

type refRepo struct {
	data *coredata.Data
}

// NewRefRepo builds the id→name resolver used to denormalize dynamic-entity
// names (roles, depts) into list/detail responses.
func NewRefRepo(data *coredata.Data) biz.RefRepo { return &refRepo{data: data} }

func (r *refRepo) RoleNames(ctx context.Context, ids []int64) (map[int64]string, error) {
	return r.names(ctx, &sysRole{}, ids)
}

func (r *refRepo) DeptNames(ctx context.Context, ids []int64) (map[int64]string, error) {
	return r.names(ctx, &sysDept{}, ids)
}

func (r *refRepo) UserNames(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID                 int64
		Username, Nickname string
	}
	if err := r.data.DB.WithContext(ctx).Model(&sysUser{}).
		Select("id, username, nickname").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Nickname
		if row.Nickname == "" {
			out[row.ID] = row.Username
		}
	}
	return out, nil
}

// names looks up id→name in a batch (missing or deleted IDs are left out of the map; the caller falls back to #id).
func (r *refRepo) names(ctx context.Context, model any, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID   int64
		Name string
	}
	if err := r.data.DB.WithContext(ctx).Model(model).
		Select("id, name").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Name
	}
	return out, nil
}
