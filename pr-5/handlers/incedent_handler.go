package handlers

import (
	"net/http"

	"notion_pr5/entity"
	"notion_pr5/entity/dto"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CreateIncidentHandler godoc
// @Summary Create an incident as the authenticated engineer
// @Tags incidents
// @Accept json
// @Produce json
// @Param request body dto.CreateIncidentDTO true "Incident data"
// @Success 201 {object} entity.Incident
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /incidents [post]
func CreateIncidentHandler(c *gin.Context, db *gorm.DB) {
	value, exists := c.Get("userID")
	userID, valid := value.(uint)
	if !exists || !valid || userID == 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized access"})
		return
	}
	var dto dto.CreateIncidentDTO

	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})

		return
	}
	result := entity.Incident{
		UserID:      userID,
		Title:       dto.Title,
		Description: dto.Description,
		Priority:    dto.Priority,
	}

	if err := db.Create(&result).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create incident"})
		return
	}
	c.JSON(http.StatusCreated, &result)
}

// FindAll godoc
// @Summary List incidents publicly
// @Tags incidents
// @Produce json
// @Success 200 {array} entity.Incident
// @Failure 500 {object} map[string]string
// @Router /incidents [get]
func FindAll(c *gin.Context, db *gorm.DB) {
	var incidents []entity.Incident

	if err := db.Find(&incidents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list incidents"})
		return
	}

	c.JSON(http.StatusOK, &incidents)
}

// FindById godoc
// @Summary Get an incident by ID
// @Tags incidents
// @Produce json
// @Param id path int true "Incident ID"
// @Success 200 {object} entity.Incident
// @Failure 404 {object} map[string]string
// @Router /incidents/{id} [get]
func FindById(ctx *gin.Context, db *gorm.DB) {
	var incident entity.Incident

	id := ctx.Param("id")

	if err := db.First(&incident, id).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, &incident)
}

// UpdateById godoc
// @Summary Update an incident status
// @Tags incidents
// @Accept json
// @Produce json
// @Param id path int true "Incident ID"
// @Param request body dto.UpdateIncidentDTO true "New status"
// @Success 200 {object} entity.Incident
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /incidents/{id}/status [patch]
func UpdateById(ctx *gin.Context, db *gorm.DB) {
	var incident entity.Incident
	var dto dto.UpdateIncidentDTO

	id := ctx.Param("id")

	if err := ctx.ShouldBindJSON(&dto); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if err := db.First(&incident, id).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	err := db.Model(&incident).Update("status", &dto.Status).Error
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, &incident)
}

// DeleteById godoc
// @Summary Delete an incident
// @Tags incidents
// @Produce json
// @Param id path int true "Incident ID"
// @Success 204 "Incident deleted"
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /incidents/{id} [delete]
func DeleteById(ctx *gin.Context, db *gorm.DB) {
	id := ctx.Param("id")

	var incident entity.Incident

	if err := db.First(&incident, id).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	db.Model(&incident).Delete(&incident, id)
	ctx.JSON(http.StatusNoContent, incident)
}
