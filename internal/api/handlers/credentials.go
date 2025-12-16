package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/api/common"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/protocols"
)

// CredentialHandler handles credential profile endpoints
type CredentialHandler struct {
	*common.CRUDHandler[dbgen.CredentialProfile]
}

func NewCredentialHandler(deps *common.Dependencies) *CredentialHandler {
	h := &CredentialHandler{}
	h.CRUDHandler = &common.CRUDHandler[dbgen.CredentialProfile]{
		Deps:      deps,
		Name:      "Credential Profile",
		CacheType: "credential",
	}

	h.ListFunc = h.list
	h.CreateFunc = h.create
	h.GetFunc = h.get
	h.UpdateFunc = h.update
	h.DeleteFunc = h.delete

	return h
}

func (h *CredentialHandler) list(ctx context.Context) ([]dbgen.CredentialProfile, error) {
	profiles, err := h.Deps.Q.ListCredentialProfiles(ctx)
	if err != nil {
		return nil, err
	}
	// Decrypt
	for i := range profiles {
		var encryptedStr string
		if err := json.Unmarshal(profiles[i].CredentialData, &encryptedStr); err == nil {
			if decrypted, err := h.Deps.Decrypt(encryptedStr); err == nil {
				profiles[i].CredentialData = decrypted
			}
		}
	}
	return profiles, nil
}

func (h *CredentialHandler) create(ctx context.Context, input dbgen.CredentialProfile) (dbgen.CredentialProfile, error) {
	if input.Name == "" || input.Protocol == "" {
		return dbgen.CredentialProfile{}, errors.New("Name and Protocol are required")
	}
	if err := validateCredentials(h.Deps.Registry, input.Protocol, input.CredentialData); err != nil {
		return dbgen.CredentialProfile{}, err
	}

	// Encrypt
	encrypted, err := h.Deps.Encrypt(input.CredentialData)
	if err != nil {
		return dbgen.CredentialProfile{}, err
	}
	// Wrap as JSON string for storage
	encryptedJSON := json.RawMessage(fmt.Sprintf("%q", encrypted))

	params := dbgen.CreateCredentialProfileParams{
		Name:           input.Name,
		Description:    input.Description,
		Protocol:       input.Protocol,
		CredentialData: encryptedJSON,
	}
	return h.Deps.Q.CreateCredentialProfile(ctx, params)
}

func (h *CredentialHandler) get(ctx context.Context, id uuid.UUID) (dbgen.CredentialProfile, error) {
	profile, err := h.Deps.Q.GetCredentialProfile(ctx, id)
	if err != nil {
		return dbgen.CredentialProfile{}, err
	}
	// Decrypt
	var encryptedStr string
	if err := json.Unmarshal(profile.CredentialData, &encryptedStr); err == nil {
		if decrypted, err := h.Deps.Decrypt(encryptedStr); err == nil {
			profile.CredentialData = decrypted
		}
	}
	return profile, nil
}

func (h *CredentialHandler) update(ctx context.Context, id uuid.UUID, input dbgen.CredentialProfile) (dbgen.CredentialProfile, error) {
	// Validate if protocol/data provided
	if input.Protocol != "" && len(input.CredentialData) > 0 {
		if err := validateCredentials(h.Deps.Registry, input.Protocol, input.CredentialData); err != nil {
			return dbgen.CredentialProfile{}, err
		}
	}

	// Encrypt if present
	if len(input.CredentialData) > 0 {
		encrypted, err := h.Deps.Encrypt(input.CredentialData)
		if err != nil {
			return dbgen.CredentialProfile{}, err
		}
		input.CredentialData = json.RawMessage(fmt.Sprintf("%q", encrypted))
	}

	params := dbgen.UpdateCredentialProfileParams{
		ID:             id,
		Name:           input.Name,
		Description:    input.Description,
		Protocol:       input.Protocol,
		CredentialData: input.CredentialData,
	}
	return h.Deps.Q.UpdateCredentialProfile(ctx, params)
}

func (h *CredentialHandler) delete(ctx context.Context, id uuid.UUID) error {
	return h.Deps.Q.DeleteCredentialProfile(ctx, id)
}

func validateCredentials(registry *protocols.Registry, protocol string, data json.RawMessage) error {
	_, err := registry.ValidateCredentials(protocol, data)
	// We could wrap ValidationErrors here if needed to make them JSON-friendly via error interface?
	// The original handler did special casting.
	// For now, returning the error is fine, the generic handler calls err.Error() which is text.
	// If we want struct-based details in API response, we might need to enhance SendError or GenericHandler to check error type.
	return err
}
