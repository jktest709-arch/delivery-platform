package bootstrap

import (
	"fmt"
	"time"

	"delivery-platform/backend/internal/auth"
	"delivery-platform/backend/internal/model"
	"gorm.io/gorm"
)

func Seed(db *gorm.DB) error {
	if err := seedUsers(db); err != nil {
		return err
	}
	if err := seedBusinessLines(db); err != nil {
		return err
	}
	if err := seedProjects(db); err != nil {
		return err
	}
	return seedDependencies(db)
}

func seedUsers(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	users := []struct {
		username string
		name     string
		role     string
		password string
	}{
		{"admin", "高远", model.RoleAdmin, "admin123"},
		{"release", "周岚", model.RoleReleaseManager, "release123"},
		{"dev", "林辰", model.RoleDeveloper, "dev123"},
	}

	for _, item := range users {
		hash, err := auth.HashPassword(item.password)
		if err != nil {
			return err
		}
		if err := db.Create(&model.User{
			Username:     item.username,
			DisplayName:  item.name,
			PasswordHash: hash,
			Role:         item.role,
			Status:       "enabled",
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedBusinessLines(db *gorm.DB) error {
	lines := []model.BusinessLine{
		{Code: "aa", Name: "AA 零售业务线", Platform: "AAPRD", TagPrefix: "aaprd", TagTemplate: "{prefix}-{timestamp}-{releaseNo}", Approver: "交易发布经理"},
		{Code: "bb", Name: "BB 履约业务线", Platform: "BBPRD", TagPrefix: "bbprd", TagTemplate: "{prefix}-{timestamp}-{releaseNo}", Approver: "履约发布经理"},
		{Code: "ops", Name: "OPS 平台业务线", Platform: "OPSPRD", TagPrefix: "opsprd", TagTemplate: "{prefix}-{timestamp}-{releaseNo}", Approver: "平台 SRE"},
	}
	for _, line := range lines {
		if err := db.Where("code = ?", line.Code).FirstOrCreate(&line).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedProjects(db *gorm.DB) error {
	lineID := map[string]uint{}
	var lines []model.BusinessLine
	if err := db.Find(&lines).Error; err != nil {
		return err
	}
	for _, line := range lines {
		lineID[line.Code] = line.ID
	}

	projects := []model.Project{
		project("base-auth", "统一认证中心", model.ProjectKindBackend, "平台组", lineID["ops"], "https://gitlab.corp/delivery/base-auth", "delivery/base-auth", "master", "build-auth-prd", "deploy-auth-prd", 10),
		project("order-core", "订单核心服务", model.ProjectKindBackend, "交易组", lineID["aa"], "https://gitlab.corp/delivery/order-core", "delivery/order-core", "master", "build-order-prd", "deploy-order-prd", 20),
		project("pay-gateway", "支付网关", model.ProjectKindBackend, "支付组", lineID["aa"], "https://gitlab.corp/delivery/pay-gateway", "delivery/pay-gateway", "master", "build-pay-prd", "deploy-pay-prd", 30),
		project("dispatch-engine", "履约调度引擎", model.ProjectKindBackend, "履约组", lineID["bb"], "https://gitlab.corp/delivery/dispatch-engine", "delivery/dispatch-engine", "master", "build-dispatch-prd", "deploy-dispatch-prd", 40),
		project("merchant-portal", "商家工作台", model.ProjectKindFrontend, "商家组", lineID["bb"], "https://gitlab.corp/delivery/merchant-portal", "delivery/merchant-portal", "main", "build-portal-prd", "deploy-portal-prd", 50),
		project("mobile-bff", "移动端 BFF", model.ProjectKindBackend, "无线组", lineID["aa"], "https://gitlab.corp/delivery/mobile-bff", "delivery/mobile-bff", "master", "build-mobile-prd", "deploy-mobile-prd", 60),
		project("reporting", "运营报表中心", model.ProjectKindFrontend, "数据组", lineID["ops"], "https://gitlab.corp/delivery/reporting", "delivery/reporting", "main", "build-report-prd", "deploy-report-prd", 70),
	}
	for _, item := range projects {
		if err := db.Where("code = ?", item.Code).FirstOrCreate(&item).Error; err != nil {
			return err
		}
	}
	return nil
}

func project(code, name, kind, owner string, lineID uint, repo, projectID, branch, packageJob, deployJob string, order int) model.Project {
	return model.Project{
		Code:            code,
		Name:            name,
		Kind:            kind,
		Owner:           owner,
		BusinessLineID:  lineID,
		GitLabURL:       repo,
		GitLabProjectID: projectID,
		DefaultBranch:   branch,
		PackageJob:      packageJob,
		DeployJob:       deployJob,
		SortOrder:       order,
		Enabled:         true,
	}
}

func seedDependencies(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.ProjectDependency{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	projectID := map[string]uint{}
	var projects []model.Project
	if err := db.Find(&projects).Error; err != nil {
		return err
	}
	for _, project := range projects {
		projectID[project.Code] = project.ID
	}

	rules := map[string][]string{
		"order-core":      []string{"base-auth"},
		"pay-gateway":     []string{"base-auth", "order-core"},
		"dispatch-engine": []string{"order-core"},
		"merchant-portal": []string{"dispatch-engine"},
		"mobile-bff":      []string{"order-core", "pay-gateway"},
		"reporting":       []string{"order-core", "dispatch-engine"},
	}
	now := time.Now()
	for code, deps := range rules {
		for _, dep := range deps {
			item := model.ProjectDependency{
				ProjectID:          projectID[code],
				DependsOnProjectID: projectID[dep],
				CreatedAt:          now,
				UpdatedAt:          now,
			}
			if item.ProjectID == 0 || item.DependsOnProjectID == 0 {
				return fmt.Errorf("invalid dependency %s -> %s", code, dep)
			}
			if err := db.Create(&item).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
