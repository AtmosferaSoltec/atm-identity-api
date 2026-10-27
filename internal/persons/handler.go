package persons

import (
	"regexp"

	"atm-identity-api/internal/platform/apierror"
	"atm-identity-api/internal/platform/apiresponse"

	"github.com/gin-gonic/gin"
)

var dniPattern = regexp.MustCompile(`^[0-9]{8}$`)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/persons/:document_number", h.get)
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
