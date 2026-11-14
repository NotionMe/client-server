package dto

type CreateIncidentDTO struct {
	Title       string `json:"title" binding:"required,min=5,max=100"`
	Description string `json:"description" binding:"required"`
	Priority    string `json:"priority" binding:"required,oneof=low medium high critical"`
}

type UpdateIncidentDTO struct {
	Status string `json:"status" binding:"required,oneof=open in_progress resolved closed"`
}
