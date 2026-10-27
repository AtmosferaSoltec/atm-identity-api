package companies

import (
	"regexp"

	"atm-identity-api/internal/platform/apierror"
	"atm-identity-api/internal/platform/apiresponse"

	"github.com/gin-gonic/gin"
)

var rucPattern = regexp.MustCompile(`^[0-9]{11}$`)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/companies/:document_number", h.get)
}

func (h *Handler) get(c *gin.Context) {
	documentNumber := c.Param("document_number")
	if !rucPattern.MatchString(documentNumber) {
		apiresponse.Fail(c, apierror.InvalidDocumentNumber("A RUC must have exactly 11 digits."))
		return
	}
	company, source, err := h.service.Resolve(c.Request.Context(), documentNumber, c.Query("refresh") == "true")
	if err != nil {
		apiresponse.Fail(c, err)
		return
	}
	apiresponse.OK(c, company, source)
}
