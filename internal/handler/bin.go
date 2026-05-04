package handler

import (
"net/http"

"github.com/beon/bin-api/internal/service"
"github.com/beon/bin-api/pkg/validator"
"github.com/gin-gonic/gin"
)

const poweredBy = "BEON API"

type BINHandler struct {
svc *service.BINService
}

func NewBINHandler(svc *service.BINService) *BINHandler {
return &BINHandler{svc: svc}
}

// Lookup handles GET /api/v1/bin/:number
func (h *BINHandler) Lookup(c *gin.Context) {
bin := c.Param("number")

if !validator.IsValidBIN(bin) {
c.JSON(http.StatusBadRequest, gin.H{
"success":    false,
"error":      "BIN must be 6-8 numeric digits",
"code":       400,
"powered_by": poweredBy,
})
return
}

result, err := h.svc.Lookup(bin)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{
"success":    false,
"error":      "Internal server error",
"code":       500,
"powered_by": poweredBy,
})
return
}
if result == nil {
c.JSON(http.StatusNotFound, gin.H{
"success":    false,
"error":      "BIN not found",
"code":       404,
"powered_by": poweredBy,
})
return
}

c.JSON(http.StatusOK, gin.H{
"success":    true,
"data":       result,
"powered_by": poweredBy,
})
}

// ValidateCard handles GET /api/v1/bin/:number/validate
// Accepts full card number and checks BIN info + Luhn validity
func (h *BINHandler) ValidateCard(c *gin.Context) {
number := c.Query("card")
if number == "" {
c.JSON(http.StatusBadRequest, gin.H{
"success":    false,
"error":      "Query param 'card' is required",
"code":       400,
"powered_by": poweredBy,
})
return
}
if len(number) < 13 || len(number) > 19 {
c.JSON(http.StatusBadRequest, gin.H{
"success":    false,
"error":      "Card number must be 13-19 digits",
"code":       400,
"powered_by": poweredBy,
})
return
}

luhnValid := validator.Luhn(number)

// Extract BIN from card number (first 6 digits)
bin := number[:6]
result, err := h.svc.Lookup(bin)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{
"success":    false,
"error":      "Internal server error",
"code":       500,
"powered_by": poweredBy,
})
return
}

c.JSON(http.StatusOK, gin.H{
"success":    true,
"luhn_valid": luhnValid,
"bin_info":   result,
"powered_by": poweredBy,
})
}

// Stats handles GET /api/v1/stats
func (h *BINHandler) Stats(c *gin.Context) {
count, err := h.svc.Stats()
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{
"success":    false,
"error":      "Failed to get stats",
"code":       500,
"powered_by": poweredBy,
})
return
}
c.JSON(http.StatusOK, gin.H{
"success":    true,
"total_bins": count,
"powered_by": poweredBy,
})
}
