package types

import (
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DatabaseService 数据库服务
type DatabaseService struct {
	db  *gorm.DB
	cfg *Config
}

// NewDatabaseService 创建新的数据库服务
func NewDatabaseService(cfg *Config) (*DatabaseService, error) {
	db, err := gorm.Open(sqlite.Open(cfg.SqlitePath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// 自动迁移数据库结构
	db.AutoMigrate(&SubmitCtx{})
	db.AutoMigrate(&User{})

	// 清理未完成的提交
	db.Model(&SubmitCtx{}).Where("status != ? AND status != ? AND status != ?", "completed", "dead", "failed").Update("status", "dead")

	return &DatabaseService{
		db:  db,
		cfg: cfg,
	}, nil
}

// GetDB 获取数据库实例
func (ds *DatabaseService) GetDB() *gorm.DB {
	return ds.db
}

// ===============================
// 用户操作
// ===============================

// CreateUser 创建新用户
func (ds *DatabaseService) CreateUser(userID string) (*User, error) {
	user := &User{
		ID:             userID,
		Token:          uuid.New().String(),
		BestScores:     make(map[string]float64),
		BestSubmits:    make(map[string]string),
		BestSubmitDate: make(map[string]int64),
		TotalScore:     0,
	}

	result := ds.db.Create(user)
	if result.Error != nil {
		return nil, result.Error
	}

	log.Info().Str("user", userID).Msg("Created new user")
	return user, nil
}

// GetUserByID 根据ID获取用户
func (ds *DatabaseService) GetUserByID(userID string) (*User, error) {
	var user User
	result := ds.db.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// 用户不存在，创建新用户
			return ds.CreateUser(userID)
		}
		return nil, result.Error
	}
	return &user, nil
}

// GetUserByToken 根据Token获取用户
func (ds *DatabaseService) GetUserByToken(token string) (*User, error) {
	var user User
	result := ds.db.Where("token = ?", token).First(&user)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

// UpdateUser 更新用户信息
func (ds *DatabaseService) UpdateUser(user *User) error {
	user.CalculateTotalScore()
	result := ds.db.Save(user)
	return result.Error
}

// GetAllUsersOrderedByScore 获取按分数排序的所有用户
func (ds *DatabaseService) GetAllUsersOrderedByScore() ([]User, error) {
	var users []User
	result := ds.db.Order("total_score desc").Find(&users)
	return users, result.Error
}

// UpdateUserSubmitResult 更新用户提交结果
func (ds *DatabaseService) UpdateUserSubmitResult(userID string, submit *SubmitCtx, problem *Problem) error {
	user, err := ds.GetUserByID(userID)
	if err != nil {
		return err
	}

	if submit.Status == "completed" && submit.JudgeResult.Success {
		newScore := submit.JudgeResult.Score * problem.Weight
		if user.BestScores[submit.Problem] < newScore {
			user.BestScores[submit.Problem] = newScore
			user.BestSubmits[submit.Problem] = submit.ID
			user.BestSubmitDate[submit.Problem] = submit.SubmitTime
		}
	}

	return ds.UpdateUser(user)
}

// DoFullUserScan 全量用户扫描和重计算
func (ds *DatabaseService) DoFullUserScan(problems map[string]Problem) error {
	var submits []SubmitCtx
	ds.db.Find(&submits)

	var users []User
	ds.db.Find(&users)

	userMap := make(map[string]User)
	for _, user := range users {
		userMap[user.ID] = user
	}

	for _, s := range submits {
		u, ok := userMap[s.User]
		if !ok {
			log.Fatal().Msg("Encountered corrupted data, submitted user does not exist in User table")
		}

		if s.Status == "completed" && s.JudgeResult.Success {
			problem, exists := problems[s.Problem]
			if exists {
				newScore := s.JudgeResult.Score * problem.Weight
				if u.BestScores[s.Problem] < newScore {
					u.BestScores[s.Problem] = newScore
					u.BestSubmits[s.Problem] = s.ID
					u.BestSubmitDate[s.Problem] = s.SubmitTime
				}
			}
		}

		userMap[s.User] = u
	}

	for _, u := range userMap {
		u.CalculateTotalScore()
		ds.db.Save(&u)
	}

	return nil
}

// IsAdmin 检查用户是否为管理员
func (ds *DatabaseService) IsAdmin(userID string) bool {
	for _, admin := range ds.cfg.Admins {
		if admin == userID {
			return true
		}
	}
	return false
}

// ===============================
// 提交记录操作
// ===============================

// CreateSubmit 创建新提交
func (ds *DatabaseService) CreateSubmit(submit *SubmitCtx) error {
	submit.LastUpdate = time.Now().UnixNano()
	result := ds.db.Create(submit)
	return result.Error
}

// UpdateSubmit 更新提交记录
func (ds *DatabaseService) UpdateSubmit(submit *SubmitCtx) error {
	submit.LastUpdate = time.Now().UnixNano()
	result := ds.db.Save(submit)
	return result.Error
}

// GetSubmitByID 根据ID获取提交记录
func (ds *DatabaseService) GetSubmitByID(submitID string) (*SubmitCtx, error) {
	var submit SubmitCtx
	result := ds.db.Where("id = ?", submitID).First(&submit)
	if result.Error != nil {
		return nil, result.Error
	}
	return &submit, nil
}

// GetSubmitsByUser 获取用户的提交记录（分页）
func (ds *DatabaseService) GetSubmitsByUser(userID string, page, limit int) ([]SubmitCtx, int64, error) {
	var submits []SubmitCtx
	var total int64

	// 获取总数
	ds.db.Model(&SubmitCtx{}).Where("user = ?", userID).Count(&total)

	// 获取分页数据
	result := ds.db.Where("user = ?", userID).
		Order("id desc").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&submits)

	return submits, total, result.Error
}

// GetAllSubmits 获取所有提交记录（分页）
func (ds *DatabaseService) GetAllSubmits(page, limit int) ([]SubmitCtx, int64, error) {
	var submits []SubmitCtx
	var total int64

	// 获取总数
	ds.db.Model(&SubmitCtx{}).Count(&total)

	// 获取分页数据
	result := ds.db.Order("id desc").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&submits)

	return submits, total, result.Error
}

// GetSubmitsForAPI 获取API用的提交记录（分页，只包含基本信息）
func (ds *DatabaseService) GetSubmitsForAPI(page, limit int) ([]SubmitCtx, int64, error) {
	var submits []SubmitCtx
	var total int64

	// 获取总数
	ds.db.Model(&SubmitCtx{}).Count(&total)

	// 获取分页数据，只选择需要的字段
	result := ds.db.Select("id", "user", "problem", "submit_time", "status", "msg", "judge_result").
		Order("id desc").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&submits)

	return submits, total, result.Error
}

// FindSubmitsByUserAndPattern 根据用户和模式查找提交（用于模糊搜索）
func (ds *DatabaseService) FindSubmitsByUserAndPattern(userID, pattern string) (*SubmitCtx, error) {
	var submit SubmitCtx
	result := ds.db.Order("id desc").
		Where("id LIKE ? AND user = ?", "%"+pattern+"%", userID).
		First(&submit)
	if result.Error != nil {
		return nil, result.Error
	}
	return &submit, nil
}

// GetSubmitCount 获取提交总数
func (ds *DatabaseService) GetSubmitCount() (int64, error) {
	var count int64
	result := ds.db.Model(&SubmitCtx{}).Count(&count)
	return count, result.Error
}

// GetUserSubmitCount 获取用户提交总数
func (ds *DatabaseService) GetUserSubmitCount(userID string) (int64, error) {
	var count int64
	result := ds.db.Model(&SubmitCtx{}).Where("user = ?", userID).Count(&count)
	return count, result.Error
}

// HasUserRunningSubmit 检查用户是否有运行中的提交
func (ds *DatabaseService) HasUserRunningSubmit(userID string) (bool, error) {
	var count int64
	result := ds.db.Model(&SubmitCtx{}).
		Where("user = ? AND status != ? AND status != ? AND status != ?", userID, "completed", "failed", "dead").
		Count(&count)
	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

// GetUserRunningSubmit 获取用户当前运行中的提交
func (ds *DatabaseService) GetUserRunningSubmit(userID string) (*SubmitCtx, error) {
	var submit SubmitCtx
	result := ds.db.Where("user = ? AND status != ? AND status != ? AND status != ?", userID, "completed", "failed", "dead").
		Order("id desc").
		First(&submit)
	if result.Error != nil {
		return nil, result.Error
	}
	return &submit, nil
}

// DeleteOldSubmits 删除旧的提交记录（可选功能）
func (ds *DatabaseService) DeleteOldSubmits(beforeTime time.Time) error {
	result := ds.db.Where("submit_time < ?", beforeTime.UnixNano()).Delete(&SubmitCtx{})
	return result.Error
}

// DeleteSubmitByID 删除指定ID的提交记录（简单版本，不重新计算权重）
func (ds *DatabaseService) DeleteSubmitByID(submitID string) error {
	result := ds.db.Where("id = ?", submitID).Delete(&SubmitCtx{})
	return result.Error
}

// DeleteSubmitByIDWithProblems 删除指定ID的提交记录并更新用户加权分数
func (ds *DatabaseService) DeleteSubmitByIDWithProblems(submitID string, problems map[string]Problem) error {
	// 先获取要删除的提交记录信息
	submit, err := ds.GetSubmitByID(submitID)
	if err != nil {
		return err
	}

	// 删除提交记录
	result := ds.db.Where("id = ?", submitID).Delete(&SubmitCtx{})
	if result.Error != nil {
		return result.Error
	}

	// 重新计算受影响用户的最佳记录（包含权重计算）
	return ds.RecalculateUserBestScoresWithProblems(submit.User, problems)
}

// RecalculateUserBestScoresWithProblems 重新计算用户的最佳分数和提交记录（包含权重）
func (ds *DatabaseService) RecalculateUserBestScoresWithProblems(userID string, problems map[string]Problem) error {
	// 获取用户
	user, err := ds.GetUserByID(userID)
	if err != nil {
		return err
	}

	// 重置用户的最佳记录
	user.BestScores = make(map[string]float64)
	user.BestSubmits = make(map[string]string)
	user.BestSubmitDate = make(map[string]int64)

	// 获取用户的所有完成的提交
	var submits []SubmitCtx
	ds.db.Where("user = ? AND status = ?", userID, "completed").Find(&submits)

	// 重新计算每个问题的最佳分数
	for _, submit := range submits {
		if submit.JudgeResult.Success {
			problemID := submit.Problem
			problem, exists := problems[problemID]
			if !exists {
				continue // 跳过不存在的问题
			}

			weightedScore := submit.JudgeResult.Score * problem.Weight
			currentBest, exists := user.BestScores[problemID]

			if !exists || weightedScore > currentBest {
				user.BestScores[problemID] = weightedScore
				user.BestSubmits[problemID] = submit.ID
				user.BestSubmitDate[problemID] = submit.SubmitTime
			}
		}
	}

	user.CalculateTotalScore()

	// 保存更新后的用户记录
	return ds.UpdateUser(user)
}

// ModifySubmissionResult 修改提交结果并更新用户记录
func (ds *DatabaseService) ModifySubmissionResult(submitID string, score float64, message string, problems map[string]Problem) error {
	// 获取提交记录
	submit, err := ds.GetSubmitByID(submitID)
	if err != nil {
		return err
	}

	// 更新提交结果
	submit.JudgeResult.Score = score
	submit.JudgeResult.Success = score > 0
	submit.Msg = message
	submit.Status = "completed"

	// 保存更新后的提交记录
	err = ds.UpdateSubmit(submit)
	if err != nil {
		return err
	}

	// 重新计算用户的最佳分数
	return ds.RecalculateUserBestScoresWithProblems(submit.User, problems)
}

// GetSubmitStatistics 获取提交统计信息
func (ds *DatabaseService) GetSubmitStatistics() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// 总提交数
	var totalSubmits int64
	ds.db.Model(&SubmitCtx{}).Count(&totalSubmits)
	stats["total_submits"] = totalSubmits

	// 成功提交数
	var successSubmits int64
	ds.db.Model(&SubmitCtx{}).Where("status = ?", "completed").Count(&successSubmits)
	stats["success_submits"] = successSubmits

	// 失败提交数
	var failedSubmits int64
	ds.db.Model(&SubmitCtx{}).Where("status = ?", "failed").Count(&failedSubmits)
	stats["failed_submits"] = failedSubmits

	// 总用户数
	var totalUsers int64
	ds.db.Model(&User{}).Count(&totalUsers)
	stats["total_users"] = totalUsers

	return stats, nil
}
