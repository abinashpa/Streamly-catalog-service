package grpc

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"streamly/catalog-service/cache"
	"streamly/catalog-service/config"
	"streamly/catalog-service/models"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

type Server struct {
	db    *gorm.DB
	cache *cache.CacheManager
	cfg   config.Config
}

func NewServer(db *gorm.DB, cache *cache.CacheManager, cfg config.Config) *Server {
	return &Server{db: db, cache: cache, cfg: cfg}
}

func (s *Server) Start(port string) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()

	serviceDesc := grpc.ServiceDesc{
		ServiceName: "catalog.CatalogService",
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "GetCatalogItem",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					in := new(struct{ ID string })
					if err := dec(in); err != nil {
						return nil, err
					}
					var item models.CatalogItem
					if err := s.db.First(&item, "id = ? OR media_id = ?", in.ID, in.ID).Error; err != nil {
						return nil, fmt.Errorf("catalog item not found")
					}
					return map[string]interface{}{"item": item}, nil
				},
			},
			{
				MethodName: "GetFeed",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					in := new(struct {
						MediaType string
						Page      int
						Limit     int
					})
					if err := dec(in); err != nil {
						return nil, err
					}
					limit := in.Limit
					if limit <= 0 || limit > 50 {
						limit = 20
					}
					page := in.Page
					if page <= 0 {
						page = 1
					}
					offset := (page - 1) * limit

					query := s.db.Order("created_at DESC").Limit(limit).Offset(offset)
					if in.MediaType != "" && in.MediaType != "all" {
						query = query.Where("media_type = ?", in.MediaType)
					}
					var items []models.CatalogItem
					if err := query.Find(&items).Error; err != nil {
						return nil, err
					}
					return map[string]interface{}{"items": items, "source": "db"}, nil
				},
			},
			{
				MethodName: "GetTrending",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					in := new(struct{ MediaType string })
					if err := dec(in); err != nil {
						return nil, err
					}
					query := s.db.Order("views DESC, likes DESC").Limit(20)
					if in.MediaType != "" && in.MediaType != "all" {
						query = query.Where("media_type = ?", in.MediaType)
					}
					var items []models.CatalogItem
					if err := query.Find(&items).Error; err != nil {
						return nil, err
					}
					return map[string]interface{}{"items": items}, nil
				},
			},
			{
				MethodName: "RecordView",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					in := new(struct{ MediaID string })
					if err := dec(in); err != nil {
						return nil, err
					}
					var item models.CatalogItem
					if err := s.db.First(&item, "id = ? OR media_id = ?", in.MediaID, in.MediaID).Error; err != nil {
						return nil, err
					}
					_, _ = s.cache.IncrementView(ctx, item.ID)
					s.db.Model(&item).UpdateColumn("views", gorm.Expr("views + 1"))
					return map[string]interface{}{"media_id": item.ID, "views": item.Views + 1}, nil
				},
			},
			{
				MethodName: "ToggleLike",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					in := new(struct {
						MediaID string
						UserID  string
					})
					if err := dec(in); err != nil {
						return nil, err
					}
					var item models.CatalogItem
					if err := s.db.First(&item, "id = ? OR media_id = ?", in.MediaID, in.MediaID).Error; err != nil {
						return nil, err
					}
					var existing models.MediaLike
					liked := false
					if err := s.db.Where("catalog_id = ? AND user_id = ?", item.ID, in.UserID).First(&existing).Error; err == nil {
						s.db.Delete(&existing)
						s.db.Model(&item).UpdateColumn("likes", gorm.Expr("GREATEST(likes - 1, 0)"))
					} else {
						s.db.Create(&models.MediaLike{CatalogID: item.ID, UserID: in.UserID})
						s.db.Model(&item).UpdateColumn("likes", gorm.Expr("likes + 1"))
						liked = true
					}
					s.cache.Invalidate(ctx, "catalog:*")
					return map[string]interface{}{"liked": liked}, nil
				},
			},
			{
				MethodName: "GetComments",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					in := new(struct{ MediaID string })
					if err := dec(in); err != nil {
						return nil, err
					}
					var comments []models.Comment
					s.db.Where("catalog_id = ?", in.MediaID).Order("created_at DESC").Find(&comments)
					return map[string]interface{}{"comments": comments, "count": len(comments)}, nil
				},
			},
			{
				MethodName: "AddComment",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					in := new(struct {
						MediaID    string
						Content    string
						UserID     string
						UserName   string
						UserAvatar string
					})
					if err := dec(in); err != nil {
						return nil, err
					}
					userName := in.UserName
					if userName == "" {
						userName = "Streamly User"
					}
					comment := models.Comment{
						ID:         uuid.New().String(),
						CatalogID:  in.MediaID,
						UserID:     in.UserID,
						UserName:   userName,
						UserAvatar: in.UserAvatar,
						Content:    in.Content,
						CreatedAt:  time.Now(),
					}
					if err := s.db.Create(&comment).Error; err != nil {
						return nil, err
					}
					return map[string]interface{}{"comment": comment}, nil
				},
			},
		},
	}

	grpcServer.RegisterService(&serviceDesc, s)

	log.Printf("catalog-service gRPC server listening on port %s", port)
	return grpcServer.Serve(lis)
}

// FetchMediaListViaGRPC dials media-service on port 50051 via gRPC
func FetchMediaListViaGRPC(mediaServiceAddr string) ([]models.CatalogItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, mediaServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to dial media-service gRPC: %v", err)
	}
	defer conn.Close()

	var out struct {
		Items []models.CatalogItem
	}
	err = conn.Invoke(ctx, "/media.MediaService/ListMedia", map[string]interface{}{"limit": 100}, &out)
	if err != nil {
		return nil, err
	}
	return out.Items, nil
}
