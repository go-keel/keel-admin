package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

var _ biz.DeptRepo = (*deptRepo)(nil)

type deptRepo struct {
	data *coredata.Data
}

// NewDeptRepo builds the department repository.
func NewDeptRepo(data *coredata.Data) biz.DeptRepo { return &deptRepo{data: data} }

func (r *deptRepo) List(ctx context.Context) ([]*biz.Dept, error) {
	var ms []sysDept
	if err := r.data.DB.WithContext(ctx).Order("sort asc, id asc").Find(&ms).Error; err != nil {
		return nil, err
	}
	counts, err := r.userCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*biz.Dept, 0, len(ms))
	for i := range ms {
		d := ms[i].toBiz()
		d.UserCount = counts[d.ID]
		out = append(out, d)
	}
	return out, nil
}

// userCounts gets the user count of every department in one GROUP BY instead of asking once per department.
func (r *deptRepo) userCounts(ctx context.Context) (map[int64]int, error) {
	var rows []struct {
		DeptID int64
		N      int
	}
	if err := r.data.DB.WithContext(ctx).Model(&sysUser{}).
		Select("dept_id, COUNT(*) AS n").Group("dept_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[int64]int, len(rows))
	for _, row := range rows {
		out[row.DeptID] = row.N
	}
	return out, nil
}

func (r *deptRepo) Get(ctx context.Context, id int64) (*biz.Dept, error) {
	var m sysDept
	err := r.data.DB.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrDeptNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toBiz(), nil
}

func (r *deptRepo) Create(ctx context.Context, d *biz.Dept) error {
	m := sysDept{ParentID: d.ParentID, Ancestors: d.Ancestors, Name: d.Name, Sort: d.Sort, Status: d.Status}
	if err := r.data.DB.WithContext(ctx).Create(&m).Error; err != nil {
		return err
	}
	d.ID = m.ID
	return nil
}

func (r *deptRepo) Update(ctx context.Context, d *biz.Dept) error {
	return r.data.DB.WithContext(ctx).Model(&sysDept{}).Where("id = ?", d.ID).
		Updates(map[string]any{"name": d.Name, "sort": d.Sort, "status": d.Status}).Error
}

func (r *deptRepo) Delete(ctx context.Context, id int64) error {
	// sysDept carries gorm.DeletedAt, so Delete only updates deleted_at.
	return r.data.DB.WithContext(ctx).Where("id = ?", id).Delete(&sysDept{}).Error
}

func (r *deptRepo) HasChildren(ctx context.Context, id int64) (bool, error) {
	var n int64
	err := r.data.DB.WithContext(ctx).Model(&sysDept{}).Where("parent_id = ?", id).Count(&n).Error
	return n > 0, err
}

func (r *deptRepo) HasUsers(ctx context.Context, id int64) (bool, error) {
	var n int64
	err := r.data.DB.WithContext(ctx).Model(&sysUser{}).Where("dept_id = ?", id).Count(&n).Error
	return n > 0, err
}
