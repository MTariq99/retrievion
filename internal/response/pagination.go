package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mtariq99/retrievion/models"
)

func WritePaginatedSuccessResponse(c *gin.Context, message string, data interface{}, pagination models.PaginationResponse) {
	c.JSON(http.StatusOK, gin.H{
		"message":    message,
		"status":     http.StatusOK,
		"data":       data,
		"pagination": pagination,
	})
}
