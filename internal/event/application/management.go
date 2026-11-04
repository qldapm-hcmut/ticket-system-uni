package application

import (
	"context"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/event/domain"
)

type ManagementFilter struct {
	Page, PageSize         int32
	Status, Search, Method string
}
type ManagementPage struct {
	Items []map[string]any
	Total int64
}
type ManagementReader interface {
	ManagementList(context.Context, string, string, string, ManagementFilter) (ManagementPage, error)
}
type ManagementService struct{ reader ManagementReader }

func NewManagementService(r ManagementReader) *ManagementService { return &ManagementService{r} }
func (s *ManagementService) List(ctx context.Context, actor, event, kind string, f ManagementFilter) (ManagementPage, error) {
	if kind != "registrations" && kind != "check-ins" && kind != "approval-logs" {
		return ManagementPage{}, domain.ErrValidation
	}
	if kind != "approval-logs" && (f.Page < 1 || f.PageSize < 1 || f.PageSize > 100 || (int64(f.Page)-1)*int64(f.PageSize) > 2147483647) {
		return ManagementPage{}, domain.ErrValidation
	}
	validStatus := map[string]bool{"": true, "REGISTERED": true, "WAITLISTED": true, "CANCELLED": true, "CANCELLED_BY_EVENT": true, "EXPIRED": true}
	if !validStatus[f.Status] || len(f.Search) > 200 || (f.Method != "" && f.Method != "QR" && f.Method != "CODE" && f.Method != "MANUAL") {
		return ManagementPage{}, domain.ErrValidation
	}
	return s.reader.ManagementList(ctx, actor, event, kind, f)
}
