package financials

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (d *dependencies) HandleCoverage(c *gin.Context) {
	coverage, err := GetCoverage(c.Request.Context(), d.db)
	if err != nil {
		financialError(c, err)
		return
	}
	c.JSON(http.StatusOK, coverage)
}

func (d *dependencies) HandlePeriods(c *gin.Context) {
	periods, err := ListPeriods(c.Request.Context(), d.db)
	if err != nil {
		financialError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"periods": periods})
}

func (d *dependencies) HandleFacts(c *gin.Context) {
	period, ok := requestedPeriod(c)
	if !ok {
		return
	}
	facts, err := ListApprovedFacts(c.Request.Context(), d.db, period)
	if err != nil {
		financialError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"period": period, "facts": facts})
}

func (d *dependencies) HandleMetrics(c *gin.Context) {
	period, ok := requestedPeriod(c)
	if !ok {
		return
	}
	metrics, err := CalculateMetrics(c.Request.Context(), d.db, period)
	if err != nil {
		financialError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"period": period, "metrics": metrics})
}

func (d *dependencies) HandlePrompts(c *gin.Context) {
	period, ok := requestedPeriod(c)
	if !ok {
		return
	}
	prompts, err := ListReviewPrompts(c.Request.Context(), d.db, period)
	if err != nil {
		financialError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"period": period, "prompts": prompts})
}

func requestedPeriod(c *gin.Context) (string, bool) {
	period := c.Query("period")
	if _, _, kind := periodBounds(period); kind == "unknown" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "period must be YYYY or YYYY-MM"})
		return "", false
	}
	return period, true
}

func financialError(c *gin.Context, err error) {
	_ = c.Error(fmt.Errorf("BBCA financial API: %w", err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}
