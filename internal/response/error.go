package response

import (
	"github.com/gin-gonic/gin"
	"github.com/mtariq99/retrievion/models"
)

func WriteErrorResponse(c *gin.Context, message string, status int) {
	c.JSON(status, models.Response{
		Message: message,
		Data:    []interface{}{},
		Status:  status,
	})
}
