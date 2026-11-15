package router

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type dependencies struct {
	logger            *zap.Logger
	requestLog        gin.HandlerFunc
	requestBodyLimit  gin.HandlerFunc
	readiness         gin.HandlerFunc
	example           gin.HandlerFunc
	financialCoverage gin.HandlerFunc
	financialPeriods  gin.HandlerFunc
	financialFacts    gin.HandlerFunc
	financialMetrics  gin.HandlerFunc
	financialPrompts  gin.HandlerFunc
}

type DependenciesConfig struct {
	Logger            *zap.Logger
	RequestLog        gin.HandlerFunc
	RequestBodyLimit  gin.HandlerFunc
	Readiness         gin.HandlerFunc
	Example           gin.HandlerFunc
	FinancialCoverage gin.HandlerFunc
	FinancialPeriods  gin.HandlerFunc
	FinancialFacts    gin.HandlerFunc
	FinancialMetrics  gin.HandlerFunc
	FinancialPrompts  gin.HandlerFunc
}

func NewDependencies(deps *DependenciesConfig) *dependencies {
	return &dependencies{
		logger:            deps.Logger,
		requestLog:        deps.RequestLog,
		requestBodyLimit:  deps.RequestBodyLimit,
		readiness:         deps.Readiness,
		example:           deps.Example,
		financialCoverage: deps.FinancialCoverage,
		financialPeriods:  deps.FinancialPeriods,
		financialFacts:    deps.FinancialFacts,
		financialMetrics:  deps.FinancialMetrics,
		financialPrompts:  deps.FinancialPrompts,
	}
}
