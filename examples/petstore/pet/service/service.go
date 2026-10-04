// Package service holds the pet service: the operations of the API.
package service

import (
	"context"

	"example.com/petstore/pet/domain"
	"example.com/petstore/pet/protocol"
)

// PetService is a service because it has a Name() method that returns a string:
// that string starts the route of each of its exported methods, here /api/pet/.
type PetService struct{}

// Name is the name of the service.
func (PetService) Name() string { return "pet" }

// Get returns one pet.
//
// The first line of a doc comment is the summary of the operation. The text after
// "@desc:" is its description, and runs until the next annotation.
//
// @desc: Looks a pet up by its id.
//
// The pet is returned whatever its status.
//
// @tags: public
// @doc: Adoption guide: https://example.com/guides/adoption
// @response:404,NotFoundErr,There is no pet with that id
func (PetService) Get(ctx context.Context, req *protocol.GetPetReq) (*domain.Pet, error) {
	return nil, nil
}

// List returns a page of pets.
//
// @tags: public
// @method: GET
func (PetService) List(ctx context.Context, req *protocol.ListPetsReq) (*protocol.ListPetsResp, error) {
	return nil, nil
}

// Search finds pets by name.
//
// @tags: public
// @path: search
func (PetService) Search(ctx context.Context, req *protocol.SearchReq) (*protocol.ListPetsResp, error) {
	return nil, nil
}

// Create adds a pet.
//
// Only the staff can create pets. The operation is not in the public document
// because it does not have the public tag.
//
// @tags: staff
// @permission: pet:write
// @response:429,RateLimitErr,Too many pets created in a short time
func (PetService) Create(ctx context.Context, req *protocol.CreatePetReq) (*domain.Pet, error) {
	return nil, nil
}

// UploadPhoto stores a photo of a pet.
//
// @tags: staff
// @permission: pet:write
func (PetService) UploadPhoto(ctx context.Context, req *protocol.UploadPhotoReq) (*protocol.UploadPhotoResp, error) {
	return nil, nil
}

// DownloadPhoto returns the photo of a pet as an image.
//
// @tags: public
// @method: GET
// @response:404,NotFoundErr,The pet has no photo
func (PetService) DownloadPhoto(ctx context.Context, req *protocol.DownloadPhotoReq) (*protocol.DownloadPhotoResp, error) {
	return nil, nil
}

// Audit returns who changed a pet and when.
//
// @tags: staff
// @permission: pet:audit
func (PetService) Audit(ctx context.Context, req *protocol.GetPetReq) (*protocol.AuditResp, error) {
	return nil, nil
}

// ImportReq is declared next to the service, outside the packages whose types
// docgen documents, so docgen cannot describe it; overlay.yaml does.
type ImportReq struct {
	Source string
	Pets   []domain.Pet
}

// Import adds many pets at once.
//
// @tags: staff
// @permission: pet:write
func (PetService) Import(ctx context.Context, req *ImportReq) (*protocol.ListPetsResp, error) {
	return nil, nil
}

// SetStore wires the service to its storage when the program starts.
//
// It is exported Go but not an operation: "@apidoc: -" keeps it out of the
// router and out of the documents.
//
// @apidoc: -
func (PetService) SetStore(store any) {}
