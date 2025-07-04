package ui

import (
	"github.com/mrhaoxx/SOJ/types"
)

// UserManager 用户管理器 (现在只是types.DatabaseService的包装器)
type UserManager struct {
	dbService *types.DatabaseService
}

// NewUserManager 创建新的用户管理器
func NewUserManager(dbService *types.DatabaseService) *UserManager {
	return &UserManager{
		dbService: dbService,
	}
}

// DoFULLUserScan 全量用户扫描
func (um *UserManager) DoFULLUserScan(problems map[string]types.Problem) error {
	return um.dbService.DoFullUserScan(problems)
}

// UserUpdate 更新用户信息
func (um *UserManager) UserUpdate(user string, s types.SubmitCtx, problem *types.Problem) error {
	return um.dbService.UpdateUserSubmitResult(user, &s, problem)
}

// GetToken 获取用户token
func (um *UserManager) GetToken(user string) string {
	u, err := um.dbService.GetUserByID(user)
	if err != nil {
		return ""
	}
	return u.Token
}

// IsAdmin 检查是否为管理员
func (um *UserManager) IsAdmin(user string) bool {
	return um.dbService.IsAdmin(user)
}
