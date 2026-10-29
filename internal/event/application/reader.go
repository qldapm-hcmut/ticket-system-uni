package application

import "context"

type Filter struct {
	UnitID, Status, Search string
	Page, PageSize         int32
}
type Reader interface {
	Units(context.Context, string) ([]map[string]any, error)
	Detail(context.Context, string) (map[string]any, error)
	Managed(context.Context, Actor, Filter) ([]map[string]any, int64, error)
}
