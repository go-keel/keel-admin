package service

import (
	"context"
	"sort"
	"time"

	"github.com/nicksnyder/go-i18n/v2/i18n"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel/middleware/locale"
)

// MetaService implements api.keel_admin.v1.MetaService.
type MetaService struct {
	v1.UnsafeMetaServiceServer
	businessZone *time.Location
}

// NewMetaService builds the meta service handler. businessZone is the time zone parsed from
// sys.business_time_zone (see server.ProvideBusinessZone).
func NewMetaService(businessZone *time.Location) *MetaService {
	return &MetaService{businessZone: businessZone}
}

// ListPermissions returns every operation plus the group display names, ordered by code.
//
// The order is required: the registry is a map, so iterating it directly gives a different order on every request,
// the checkboxes on the configuration page would jump around on refresh, and finding a code would mean scanning
// again. A code is business-domain:module:operation itself, so sorting by it sorts by the three-level structure.
//
// Display names are translated per request language (the key is derived from the code, see perm.Code.NameKey), the
// same server-side localization as resource names and audit action names. A missing message falls back to the slug,
// never to text hard-coded in another language.
func (s *MetaService) ListPermissions(ctx context.Context, _ *v1.ListPermissionsRequest) (*v1.ListPermissionsReply, error) {
	all := perm.All()
	items := make([]*v1.PermissionDef, 0, len(all))
	for _, p := range all {
		deps := make([]string, len(p.Deps))
		for i, d := range p.Deps {
			deps[i] = string(d)
		}
		items = append(items, &v1.PermissionDef{
			Code:       string(p.Code),
			Name:       displayName(ctx, p.Code.NameKey(), p.Code.Action()),
			Domain:     p.Code.Domain(),
			Module:     p.Code.Module(),
			Action:     p.Code.Action(),
			Deps:       deps,
			RequireOtp: p.RequireOTP,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].GetCode() < items[j].GetCode() })

	defs := perm.Groups()
	groups := make([]*v1.PermGroup, 0, len(defs))
	domainSort := make(map[string]int, len(defs))
	domainName := make(map[string]string, len(defs))
	for _, g := range defs {
		// A missing message falls back to this level's slug: the domain name for a business domain, the module name for a
		// module.
		slug := g.Domain
		if g.Module != "" {
			slug = g.Module
		}
		name := displayName(ctx, g.NameKey(), slug)
		groups = append(groups, &v1.PermGroup{
			Domain: g.Domain, Module: g.Module, Name: name, Sort: int32(g.Sort),
		})
		if g.Module == "" {
			domainSort[g.Domain] = g.Sort
			domainName[g.Domain] = name
		}
	}
	// The same order as the frontend catalogue: business domains by domain Sort and display name; within a domain the
	// domain node comes first, then the modules by module Sort and display name. sys uses a huge Sort, so it reliably
	// ends up after every business domain.
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.GetDomain() != b.GetDomain() {
			as, bs := domainSort[a.GetDomain()], domainSort[b.GetDomain()]
			if as != bs {
				return as < bs
			}
			an, bn := domainName[a.GetDomain()], domainName[b.GetDomain()]
			if an != bn {
				return an < bn
			}
			return a.GetDomain() < b.GetDomain()
		}
		if a.GetModule() == "" || b.GetModule() == "" {
			return a.GetModule() == "" && b.GetModule() != ""
		}
		if a.GetSort() != b.GetSort() {
			return a.GetSort() < b.GetSort()
		}
		if a.GetName() != b.GetName() {
			return a.GetName() < b.GetName()
		}
		return a.GetModule() < b.GetModule()
	})
	return &v1.ListPermissionsReply{Items: items, Groups: groups}, nil
}

// ListResources exposes the data scope resource registry: the UI turns resource types into a dropdown from it and
// decides by anchor which scope_type values are selectable (the decision still lives in biz.scopeAnchorsCompatible;
// this only brings the same fact forward to selection time, so a configuration is not rejected after it is filled
// in).
func (s *MetaService) ListResources(ctx context.Context, _ *v1.ListResourcesRequest) (*v1.ListResourcesReply, error) {
	// Resource display names are translated per request language (the same server-side localization as error
	// messages). A non-HTTP context has no localizer; name then stays empty and the frontend falls back to the slug.
	localizer, hasLocale := locale.FromContext(ctx)
	defs := perm.Resources()
	items := make([]*v1.ResourceDef, 0, len(defs))
	for _, d := range defs {
		it := &v1.ResourceDef{
			Res:        string(d.Res),
			AnchorSelf: d.Anchors&perm.AnchorSelf != 0,
			AnchorDept: d.Anchors&perm.AnchorDept != 0,
		}
		localize := func(key string) string {
			if !hasLocale || key == "" {
				return ""
			}
			// When the current language lacks the message, go-i18n still falls back to the default language (en)
			// text, only adding MessageNotFoundErr (as displayName in audit.go does), so it ignores err and checks
			// only for empty text: only a key that does not exist at all (a misspelled NameKey) gives an empty
			// string, and the frontend then falls back to the slug; for a merely untranslated message, English
			// is more readable than the slug.
			s, _ := localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
			return s
		}
		it.Name = localize(d.NameKey)
		// The grantable instance options: perm.GrantOptions calls the business-registered ListGrantable callback on
		// the spot (instances are runtime business data) and enforces the MaxGrantOptions ceiling. A failing
		// callback or an exceeded ceiling fails the whole request: a configuration page given incomplete options
		// grants the wrong access, so silently swallowing or truncating is more dangerous than an error; exceeding
		// the ceiling means the resource has outgrown the design limit of explicit assignment.
		if d.ListGrantable != nil {
			it.Grantable = true
			opts, err := perm.GrantOptions(ctx, d)
			if err != nil {
				return nil, err
			}
			for _, o := range opts {
				it.GrantOptions = append(it.GrantOptions, &v1.GrantOption{Id: o.ID, Label: o.Label})
			}
		}
		if d.Via != nil {
			it.ViaParent = string(d.Via.Parent)
			it.ViaFk = d.Via.FK
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].GetRes() < items[j].GetRes() })
	return &v1.ListResourcesReply{Items: items}, nil
}

func (s *MetaService) MyPermissions(ctx context.Context, _ *v1.MyPermissionsRequest) (*v1.MyPermissionsReply, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	codes := make([]string, 0, len(p.Perms))
	for c := range p.Perms {
		codes = append(codes, string(c))
	}
	return &v1.MyPermissionsReply{IsRoot: p.IsRoot, Codes: codes}, nil
}

// SystemInfo returns system-level information that does not depend on who is logged in; any logged-in user may
// read it.
func (s *MetaService) SystemInfo(context.Context, *v1.SystemInfoRequest) (*v1.SystemInfoReply, error) {
	return &v1.SystemInfoReply{BusinessTimeZone: s.businessZone.String()}, nil
}
