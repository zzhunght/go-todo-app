package cmd

import (
	"demo-service/common"
	"demo-service/composer"
	"demo-service/middleware"
	"demo-service/proto/pb"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	sctx "github.com/viettranx/service-context"
	"github.com/viettranx/service-context/component/ginc"
	smdlw "github.com/viettranx/service-context/component/ginc/middleware"
	"github.com/viettranx/service-context/component/gormc"
	"github.com/viettranx/service-context/component/jwtc"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

func newServiceCtx() sctx.ServiceContext {
	return sctx.NewServiceContext(
		sctx.WithName("Demo Microservices"),
		sctx.WithComponent(ginc.NewGin(common.KeyCompGIN)),
		sctx.WithComponent(gormc.NewGormDB(common.KeyCompMySQL, "")),
		sctx.WithComponent(jwtc.NewJWT(common.KeyCompJWT)),
		sctx.WithComponent(NewConfig()),
	)
}

var rootCmd = &cobra.Command{
	Use:   "app",
	Short: "Start service",
	Run: func(cmd *cobra.Command, args []string) {
		serviceCtx := newServiceCtx()

		logger := sctx.GlobalLogger().GetLogger("service")

		// Make some delay for DB ready (migration)
		// remove it if you already had your own DB
		time.Sleep(time.Second * 5)

		if err := serviceCtx.Load(); err != nil {
			logger.Fatal(err)
		}

		ginComp := serviceCtx.MustGet(common.KeyCompGIN).(common.GINComponent)

		router := ginComp.GetRouter()
		router.Use(gin.Recovery(), gin.Logger(), smdlw.Recovery(serviceCtx))

		router.Use(middleware.Cors())
		router.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"data": "pong"})
		})

		go StartGRPCServices(serviceCtx)

		v1 := router.Group("/v1")

		SetupRoutes(v1, serviceCtx)

		if err := router.Run(fmt.Sprintf(":%d", ginComp.GetPort())); err != nil {
			logger.Fatal(err)
		}
	},
}

func SetupRoutes(router *gin.RouterGroup, serviceCtx sctx.ServiceContext) {

	userAPIService := composer.ComposeUserAPIService(serviceCtx)

	requireAuthMdw := middleware.RequireAuth(composer.ComposeAuthRPCClient(serviceCtx))

	router.GET("/profile", requireAuthMdw, userAPIService.GetUserProfileHdl())
}

func StartGRPCServices(serviceCtx sctx.ServiceContext) {
	configComp := serviceCtx.MustGet(common.KeyCompConf).(common.Config)
	logger := serviceCtx.Logger("grpc")

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", configComp.GetGRPCPort()))

	if err != nil {
		log.Fatal(err)
	}

	logger.Infof("GRPC Server is listening on %d ...\n", configComp.GetGRPCPort())

	s := grpc.NewServer()

	pb.RegisterUserServiceServer(s, composer.ComposeUserGRPCService(serviceCtx))

	if err := s.Serve(lis); err != nil {
		log.Fatalln(err)
	}
}

func Execute() {
	rootCmd.AddCommand(outEnvCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
