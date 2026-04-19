package cmd

import "github.com/gin-gonic/gin"

type service struct {
	engine *gin.Engine
}

func NewService() *service {
	engine := gin.New()
	return &service{
		engine: engine,
	}
}

func (s *service) Engine() *gin.Engine {
	return s.engine
}

func (s *service) startHTTP() {
	s.withRouter()
}
