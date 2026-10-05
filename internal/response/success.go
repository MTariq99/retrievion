package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mtariq99/retrievion/models"
)

func WriteSuccessResponse(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, models.Response{
		Message: message,
		Data:    []interface{}{data},
		Status:  http.StatusOK,
	})
}

func WriteEmptySuccessResponse(c *gin.Context, message string) {
	c.JSON(http.StatusOK, models.Response{
		Message: message,
		Data:    []interface{}{},
		Status:  http.StatusOK,
	})
}
