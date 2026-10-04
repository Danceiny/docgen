// Package protocol holds the requests and responses of the pet service.
package protocol

import "example.com/petstore/pet/domain"

// AuthHeader is the header every request of the API carries.
type AuthHeader struct {
	// Authorization is "Bearer <token>".
	Authorization string `json:"Authorization" validate:"required"`
}

// GetPetReq asks for one pet.
type GetPetReq struct {
	ID string `json:"id" example:"p-1001" validate:"required"`
}

// ListPetsReq asks for a page of pets. It is read from the URL query, which
// docgen.yaml says under request.query.
type ListPetsReq struct {
	// Status limits the list to pets in that state.
	Status domain.Status `json:"status,omitempty"`
	// PageSize is how many pets a page has, at most 100.
	PageSize int `json:"pageSize" default:"20"`
	// Cursor is the Next of the previous page; empty for the first.
	Cursor string `json:"cursor,omitempty"`
}

// ListPetsResp is a page of pets.
type ListPetsResp struct {
	Pets []domain.Pet `json:"pets"`
	// Next is the cursor of the next page; empty when this is the last.
	Next string `json:"next,omitempty"`
}

// SearchReq looks for pets by name.
type SearchReq struct {
	// Name is part of the name of the pets to find.
	Name string `json:"name" validate:"required"`
}

// CreatePetReq adds a pet.
type CreatePetReq struct {
	Name     string          `json:"name" validate:"required"`
	Category domain.Category `json:"category" validate:"required"`
	Tags     []string        `json:"tags,omitempty"`
	Fee      domain.Price    `json:"fee"`
}

// UploadPhotoReq is a multipart upload: docgen.yaml lists its parts.
type UploadPhotoReq struct{}

// UploadPhotoResp says where the photo is.
type UploadPhotoResp struct {
	URL string `json:"url"`
}

// DownloadPhotoReq asks for the photo of a pet; like ListPetsReq it is read from
// the query string.
type DownloadPhotoReq struct {
	PetID string `json:"petId"`
}

// DownloadPhotoResp stands for the image bytes the response writes: the response
// is a file, which docgen.yaml says under response.binary.
type DownloadPhotoResp struct{}

// AuditResp is the history of a pet.
type AuditResp struct {
	Entries []domain.InternalAuditEntry `json:"entries"`
}
