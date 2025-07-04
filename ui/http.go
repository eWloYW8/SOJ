package ui

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mrhaoxx/SOJ/types"
	"github.com/rs/zerolog/log"
)

// HTTPServer HTTP服务器
type HTTPServer struct {
	dbService *types.DatabaseService
}

// NewHTTPServer 创建新的HTTP服务器
func NewHTTPServer(dbService *types.DatabaseService) *HTTPServer {
	return &HTTPServer{
		dbService: dbService,
	}
}

// AuthMiddleware 认证中间件
func (s *HTTPServer) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie("token")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    0,
				"message": "Token is required",
				"data":    nil,
			})
			c.Abort()
			return
		}

		user, err := s.dbService.GetUserByToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    0,
				"message": "Invalid Token",
				"data":    nil,
			})
			c.Abort()
			return
		}

		// Save the user in the context
		c.Set("user", user.ID)
		c.Set("is_admin", s.dbService.IsAdmin(user.ID))

		c.Next()
	}
}

// listSubmits 列出提交
func (s *HTTPServer) listSubmits(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		c.JSON(400, gin.H{
			"message": "Invalid parameter: page",
		})
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit <= 0 {
		c.JSON(400, gin.H{
			"message": "Invalid parameter: limit",
		})
		return
	}

	submits, total, err := s.dbService.GetSubmitsForAPI(page, limit)
	if err != nil {
		c.JSON(500, gin.H{
			"code":    1,
			"message": "Database error",
			"data":    nil,
		})
		return
	}

	admin, _ := c.Get("is_admin")
	user, _ := c.Get("user")
	if !admin.(bool) {
		for i := range submits {
			if submits[i].User != user.(string) {
				submits[i].User = "Anonymous"
			}
		}
	}

	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"total":   total,
			"submits": submits,
		},
	})
}

// getSubmitDetail 获取提交详情
func (s *HTTPServer) getSubmitDetail(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": "Invalid parameter",
			"data":    nil,
		})
		return
	}

	submit, err := s.dbService.GetSubmitByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    1,
			"message": "Submit not found",
			"data":    nil,
		})
		return
	}

	admin, _ := c.Get("is_admin")
	user, _ := c.Get("user")
	if !admin.(bool) && submit.User != user.(string) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    1,
			"message": "You are not allowed to view this submit",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    submit,
	})
	return
}

// listRank 排行榜
func (s *HTTPServer) listRank(c *gin.Context) {
	users, err := s.dbService.GetAllUsersOrderedByScore()
	if err != nil {
		c.JSON(500, gin.H{
			"code":    1,
			"message": "Database error",
			"data":    nil,
		})
		return
	}

	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    users,
	})
}

// getUserSummary 获取用户摘要
func (s *HTTPServer) getUserSummary(c *gin.Context) {
	id, _ := c.Get("user")
	user, err := s.dbService.GetUserByID(id.(string))
	if err != nil {
		c.JSON(500, gin.H{
			"code":    1,
			"message": "Database error",
			"data":    nil,
		})
		return
	}

	c.JSON(200, gin.H{
		"code":    0,
		"message": "success",
		"data":    user,
	})
	return
}

// ServeHTTP 启动HTTP服务器
func (s *HTTPServer) ServeHTTP(addr string) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	err := router.SetTrustedProxies([]string{"127.0.0.1"})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to set trusted proxies")
		return
	}

	auth := router.Group("/api/v1", s.AuthMiddleware())
	auth.GET("rank", s.listRank)
	auth.GET("list", s.listSubmits)
	auth.GET("my", s.getUserSummary)
	auth.GET("status/:id", s.getSubmitDetail)

	go func() {
		log.Info().Str("addr", addr).Msg("HTTP server started")
		err = router.Run(addr)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to start HTTP server")
		}
	}()
}
