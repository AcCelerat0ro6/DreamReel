package interfaceshttprecommendation

import applicationrecommendation "DreamReel/internal/application/recommendation"

type Handler struct {
	service *applicationrecommendation.Service
}

func New(service *applicationrecommendation.Service) *Handler {
	return &Handler{service: service}
}
