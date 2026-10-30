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

func ErrResp(code int, err string) models.ErrorResponse {
	return models.ErrorResponse{
		Error: err,
		Code:  code,
	}
}
