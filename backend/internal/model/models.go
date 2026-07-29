package model

import "time"

const (
	RoleDeveloper      = "developer"
	RoleReleaseManager = "release_manager"
	RoleAdmin          = "admin"
)

const (
	ProjectKindBackend  = "backend"
	ProjectKindFrontend = "frontend"
)

const (
	ReleaseStatusPending       = "pending"
	ReleaseStatusTagged        = "tagged"
	ReleaseStatusBuilding      = "building"
	ReleaseStatusBuildSuccess  = "build_success"
	ReleaseStatusDeploying     = "deploying"
	ReleaseStatusDeploySuccess = "deploy_success"
	ReleaseStatusPartialFailed = "partial_failed"
)

const (
	ProjectStatusPending       = "pending"
	ProjectStatusTagged        = "tagged"
	ProjectStatusBuilding      = "building"
	ProjectStatusBuildSuccess  = "build_success"
	ProjectStatusBuildFailed   = "build_failed"
	ProjectStatusDeploying     = "deploying"
	ProjectStatusDeploySuccess = "deploy_success"
	ProjectStatusDeployFailed  = "deploy_failed"
)

type User struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Username     string    `json:"username" gorm:"size:64;uniqueIndex;not null"`
	DisplayName  string    `json:"displayName" gorm:"size:64;not null"`
	PasswordHash string    `json:"-" gorm:"size:255;not null"`
	Role         string    `json:"role" gorm:"size:32;index;not null"`
	Status       string    `json:"status" gorm:"size:16;default:enabled"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type BusinessLine struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"size:64;uniqueIndex;not null"`
	Name        string    `json:"name" gorm:"size:128;not null"`
	Platform    string    `json:"platform" gorm:"size:64;not null"`
	TagPrefix   string    `json:"tagPrefix" gorm:"size:64;not null"`
	TagTemplate string    `json:"tagTemplate" gorm:"size:128;not null"`
	Approver    string    `json:"approver" gorm:"size:64;not null"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Project struct {
	ID                uint           `json:"id" gorm:"primaryKey"`
	Code              string         `json:"code" gorm:"size:64;uniqueIndex;not null"`
	Name              string         `json:"name" gorm:"size:128;not null"`
	Kind              string         `json:"kind" gorm:"size:32;index;not null"`
	Owner             string         `json:"owner" gorm:"size:64;not null"`
	BusinessLineID    uint           `json:"businessLineId" gorm:"index;not null"`
	BusinessLine      BusinessLine   `json:"businessLine"`
	BusinessLines     []BusinessLine `json:"businessLines" gorm:"many2many:project_business_lines;"`
	GitLabURL         string         `json:"gitlabUrl" gorm:"size:255;not null"`
	GitLabProjectID   string         `json:"gitlabProjectId" gorm:"size:255;not null"`
	DefaultBranch     string         `json:"defaultBranch" gorm:"size:128;not null"`
	PackageJob        string         `json:"packageJob" gorm:"size:128;not null"`
	DeployJob         string         `json:"deployJob" gorm:"size:128;not null"`
	SortOrder         int            `json:"sortOrder" gorm:"index;not null"`
	Enabled           bool           `json:"enabled" gorm:"default:true"`
	ProjectDependency []ProjectDependency
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type ProjectBusinessLine struct {
	ProjectID      uint      `json:"projectId" gorm:"primaryKey"`
	BusinessLineID uint      `json:"businessLineId" gorm:"primaryKey"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type ProjectDependency struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	ProjectID          uint      `json:"projectId" gorm:"uniqueIndex:idx_project_dependency;not null"`
	DependsOnProjectID uint      `json:"dependsOnProjectId" gorm:"uniqueIndex:idx_project_dependency;not null"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type Release struct {
	ID            uint             `json:"id" gorm:"primaryKey"`
	BatchNo       string           `json:"batchNo" gorm:"size:64;uniqueIndex;not null"`
	ApplicantID   uint             `json:"applicantId" gorm:"index;not null"`
	Applicant     User             `json:"applicant"`
	ApproverID    *uint            `json:"approverId" gorm:"index"`
	Approver      *User            `json:"approver"`
	Status        string           `json:"status" gorm:"size:32;index;not null"`
	ReleaseWindow time.Time        `json:"releaseWindow"`
	Remark        string           `json:"remark" gorm:"size:512"`
	Projects      []ReleaseProject `json:"projects"`
	Events        []ReleaseEvent   `json:"events"`
	CreatedAt     time.Time        `json:"createdAt"`
	UpdatedAt     time.Time        `json:"updatedAt"`
}

type ReleaseProject struct {
	ID             uint         `json:"id" gorm:"primaryKey"`
	ReleaseID      uint         `json:"releaseId" gorm:"index;not null"`
	ProjectID      uint         `json:"projectId" gorm:"index;not null"`
	Project        Project      `json:"project"`
	BusinessLineID uint         `json:"businessLineId" gorm:"index;default:0"`
	BusinessLine   BusinessLine `json:"businessLine"`
	SourceType     string       `json:"sourceType" gorm:"size:32;not null"`
	SourceRef      string       `json:"sourceRef" gorm:"size:255;not null"`
	TargetTag      string       `json:"targetTag" gorm:"size:128;not null"`
	PipelineID     string       `json:"pipelineId" gorm:"size:64"`
	BuildJobID     string       `json:"buildJobId" gorm:"size:64"`
	DeployJobID    string       `json:"deployJobId" gorm:"size:64"`
	Status         string       `json:"status" gorm:"size:32;index;not null"`
	ErrorMessage   string       `json:"errorMessage" gorm:"size:512"`
	SortOrder      int          `json:"sortOrder" gorm:"index;not null"`
	CreatedAt      time.Time    `json:"createdAt"`
	UpdatedAt      time.Time    `json:"updatedAt"`
}

type ReleaseEvent struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	ReleaseID  uint      `json:"releaseId" gorm:"index;not null"`
	OperatorID *uint     `json:"operatorId" gorm:"index"`
	Operator   *User     `json:"operator"`
	Action     string    `json:"action" gorm:"size:64;not null"`
	Message    string    `json:"message" gorm:"size:512;not null"`
	CreatedAt  time.Time `json:"createdAt"`
}
