package chat

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/config"
	"github.com/gin-gonic/gin"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	prommiddleware "github.com/slok/go-http-metrics/middleware"
	ginmiddleware "github.com/slok/go-http-metrics/middleware/gin"
	"gopkg.in/olahol/melody.v1"

	doc "github.com/Tuananh165-art/NexusChat/docs/chat"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

var (
	sessCidKey = "sesscid"

	MelodyChat MelodyChatConn
)

type MelodyChatConn struct {
	*melody.Melody
}

type HttpServer struct {
	name          string
	logger        common.HttpLog
	svr           *gin.Engine
	mc            MelodyChatConn
	httpPort      string
	httpServer    *http.Server
	msgSubscriber *MessageSubscriber
	userSvc       UserService
	msgSvc        MessageService
	chanSvc       ChannelService
	forwardSvc    ForwardService
	aiSvc         AIService
	serveSwag     bool
}

func NewMelodyChatConn(config *config.Config) MelodyChatConn {
	m := melody.New()
	m.Config.MaxMessageSize = config.Chat.Message.MaxSizeByte
	MelodyChat = MelodyChatConn{
		m,
	}
	return MelodyChat
}

func NewGinServer(name string, logger common.HttpLog, config *config.Config) *gin.Engine {
	svr := gin.New()
	svr.Use(gin.Recovery())
	svr.Use(common.CorsMiddleware())
	svr.Use(common.LoggingMiddleware(logger))
	svr.Use(common.MaxAllowed(config.Chat.Http.Server.MaxConn))

	mdlw := prommiddleware.New(prommiddleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{
			Prefix: name,
		}),
	})
	svr.Use(ginmiddleware.Handler("", mdlw))
	return svr
}

func NewHttpServer(name string, logger common.HttpLog, config *config.Config, svr *gin.Engine, mc MelodyChatConn, msgSubscriber *MessageSubscriber, userSvc UserService, msgSvc MessageService, chanSvc ChannelService, forwardSvc ForwardService, aiSvc AIService) *HttpServer {
	initJWT(config)

	return &HttpServer{
		name:          name,
		logger:        logger,
		svr:           svr,
		mc:            mc,
		httpPort:      config.Chat.Http.Server.Port,
		msgSubscriber: msgSubscriber,
		userSvc:       userSvc,
		msgSvc:        msgSvc,
		chanSvc:       chanSvc,
		forwardSvc:    forwardSvc,
		aiSvc:         aiSvc,
		serveSwag:     config.Chat.Http.Server.Swag,
	}
}

func initJWT(config *config.Config) {
	common.JwtSecret = config.Chat.JWT.Secret
	common.JwtExpirationSecond = config.Chat.JWT.ExpirationSecond
}

// @title           Chat Service Swagger API
// @version         2.0
// @description     Chat service API

// @contact.name   Ming Hsu
// @contact.email  Tuananh165-art@gmail.com

// @BasePath  /api
func (r *HttpServer) RegisterRoutes() {
	r.msgSubscriber.RegisterHandler()

	chatGroup := r.svr.Group("/api/chat")
	{
		chatGroup.GET("", r.StartChat)

		forwardAuthGroup := chatGroup.Group("/forwardauth")
		forwardAuthGroup.Use(common.JWTAuth())
		{
			forwardAuthGroup.Any("", r.ForwardAuth)
		}

		usersGroup := chatGroup.Group("/users")
		usersGroup.Use(common.JWTAuth())
		{
			usersGroup.GET("", r.GetChannelUsers)
			usersGroup.GET("/online", r.GetOnlineUsers)
		}
		channelGroup := chatGroup.Group("/channel")
		channelGroup.Use(common.JWTAuth())
		{
			channelGroup.GET("/messages", r.ListMessages)
			channelGroup.GET("/pins", r.GetPinnedMessages)
			channelGroup.GET("/search", r.SearchMessages)
			channelGroup.GET("/media", r.ListMediaMessages)
			channelGroup.DELETE("", r.DeleteChannel)
		}
		roleGroup := chatGroup.Group("/role")
		roleGroup.Use(common.JWTAuth())
		{
			roleGroup.GET("", r.GetMyRole)
			roleGroup.PUT("", r.AssignRole)
		}
		notifGroup := chatGroup.Group("/notification")
		notifGroup.Use(common.JWTAuth())
		{
			notifGroup.GET("/prefs", r.GetNotificationPrefs)
			notifGroup.PUT("/prefs", r.SetNotificationPrefs)
		}
		aiGroup := chatGroup.Group("/ai")
		aiGroup.Use(common.JWTAuth())
		{
			aiGroup.POST("/rewrite", r.RewriteWithAI)
		}
	}
	r.mc.HandleMessage(r.HandleChatOnMessage)
	r.mc.HandleConnect(r.HandleChatOnConnect)
	r.mc.HandleClose(r.HandleChatOnClose)

	if r.serveSwag {
		chatGroup.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.InstanceName(doc.SwaggerInfochat.InfoInstanceName)))
	}
}

func (r *HttpServer) Run() {
	go func() {
		addr := ":" + r.httpPort
		r.httpServer = &http.Server{
			Addr:    addr,
			Handler: common.NewOtelHttpHandler(r.svr, r.name+"_http"),
		}
		r.logger.Info("http server listening", slog.String("addr", addr))
		err := r.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			r.logger.Error(err.Error())
			os.Exit(1)
		}
	}()
	go func() {
		err := r.msgSubscriber.Run()
		if err != nil {
			r.logger.Error(err.Error())
			os.Exit(1)
		}
	}()
}
func (r *HttpServer) GracefulStop(ctx context.Context) error {
	err := MelodyChat.Close()
	if err != nil {
		return err
	}
	err = r.httpServer.Shutdown(ctx)
	if err != nil {
		return err
	}
	err = r.msgSubscriber.GracefulStop()
	if err != nil {
		return err
	}
	return nil
}

func response(c *gin.Context, httpCode int, err error) {
	message := err.Error()
	c.JSON(httpCode, common.ErrResponse{
		Message: message,
	})
}
