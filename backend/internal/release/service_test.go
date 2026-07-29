package release

import (
	"testing"
	"time"

	"delivery-platform/backend/internal/model"
)

func TestRenderTagUsesSecondPrecisionTimestamp(t *testing.T) {
	line := model.BusinessLine{
		TagPrefix:   "aaprd",
		TagTemplate: "{prefix}-{timestamp}-{releaseNo}",
	}
	tag := renderTagAt(line, "042", time.Date(2026, 7, 29, 15, 4, 5, 0, time.UTC))

	if tag != "aaprd-20260729150405-042" {
		t.Fatalf("tag = %s, want aaprd-20260729150405-042", tag)
	}
}

func TestRenderTagKeepsOldDatePlaceholderSecondPrecision(t *testing.T) {
	line := model.BusinessLine{
		TagPrefix:   "bbprd",
		TagTemplate: "{prefix}-{date}-{releaseNo}",
	}
	tag := renderTagAt(line, "007", time.Date(2026, 7, 29, 9, 8, 7, 0, time.UTC))

	if tag != "bbprd-20260729090807-007" {
		t.Fatalf("tag = %s, want bbprd-20260729090807-007", tag)
	}
}
