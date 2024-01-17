package handler

import (
	"net/http"
	"strconv"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/kubeskoop/webconsole/internal/service/controller"
)

func RegisterDiagnosisHandler(g *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	g.Use(auth.MiddlewareFunc())
	g.GET("", listDiagnosis)
	g.GET("/:id", getDiagnosisByID)
	g.POST("", newDiagnosis)
}

func newDiagnosis(ctx *gin.Context) {
	task := controller.DiagnosisTask{}
	err := ctx.ShouldBindJSON(&task)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	taskID, err := controller.Service.CreateDiagnosis(task)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"task_id": taskID})
}

func getDiagnosisByID(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "id must be integer"})
	}

	task, err := controller.Service.GetDiagnosisResult(id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, task)
}

func listDiagnosis(ctx *gin.Context) {
	tasks, err := controller.Service.ListDiagnosisResult()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, tasks)
}
