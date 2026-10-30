package models

type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type PaginationResponse struct {
	Page           int   `json:"page,omitempty"`
	Limit          int   `json:"limit"`
	TotalItems     int64 `json:"total_items,omitempty"`
	TotalPages     int   `json:"total_pages,omitempty"`
	CurrentCount   int   `json:"current_count"`
	RemainingItems int64 `json:"remaining_items,omitempty"`

	HasNext     bool `json:"has_next"`
	HasPrevious bool `json:"has_previous,omitempty"`

	NextPage     *int `json:"next_page,omitempty"`
	PreviousPage *int `json:"previous_page,omitempty"`

	NextCursor string `json:"next_cursor,omitempty"`
	From       int    `json:"from"`
	To         int    `json:"to"`
}

type PaginationQuery struct {
	// Offset Pagination
	Page   int `json:"page"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`

	// Cursor Pagination
	Cursor string `json:"cursor,omitempty"`

	// Sorting
	SortBy string `json:"sort_by"`
	Order  string `json:"order"`

	// Metadata
	IsCursorPagination bool `json:"is_cursor_pagination"`
}

type IDResponse struct {
	ID string `json:"id"`
}

type PaginationRequest struct {
	Page     int `json:"page" form:"page" validate:"omitempty,min=1"`
	PageSize int `json:"pageSize" form:"pageSize" validate:"omitempty,min=1,max=100"`
}

type PaginationMeta struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
}

type ListResponse[T any] struct {
	Data []T            `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

type Response struct {
	Data    interface{} `josn:"data"`
	Message string      `json:"message"`
	Status  int         `json:"status"`
}
