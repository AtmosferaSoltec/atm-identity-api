package persons

import (
	"encoding/json"
	"regexp"

	"atm-identity-api/internal/platform/apierror"
	"atm-identity-api/internal/platform/apiresponse"
	"atm-identity-api/internal/platform/auth"

	"github.com/gin-gonic/gin"
)

var dniPattern = regexp.MustCompile(`^[0-9]{8}$`)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/persons/:document_number", h.get)
	group.PATCH("/persons/:document_number", h.patch)
}

func (h *Handler) get(c *gin.Context) {
	documentNumber := c.Param("document_number")
	if !dniPattern.MatchString(documentNumber) {
		apiresponse.Fail(c, apierror.InvalidDocumentNumber("A DNI must have exactly 8 digits."))
		return
	}
	person, source, err := h.service.Resolve(c.Request.Context(), documentNumber, c.Query("refresh") == "true")
	if err != nil {
		apiresponse.Fail(c, err)
		return
	}
	apiresponse.OK(c, person, source)
}

func (h *Handler) patch(c *gin.Context) {
	documentNumber := c.Param("document_number")
	if !dniPattern.MatchString(documentNumber) {
		apiresponse.Fail(c, apierror.InvalidDocumentNumber("A DNI must have exactly 8 digits."))
		return
	}
	var in PatchInput
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		apiresponse.Fail(c, apierror.ValidationFailed(apierror.Detail{Field: "body", Issue: "must be a JSON object with only enrichable fields"}))
		return
	}
	client, _ := c.Get(auth.ClientSystemKey)
	code, _ := client.(string)
	record, err := h.service.Patch(c.Request.Context(), documentNumber, code, in)
	if err != nil {
		apiresponse.Fail(c, err)
		return
	}
	apiresponse.OK(c, record, SourceDatabase)
}
