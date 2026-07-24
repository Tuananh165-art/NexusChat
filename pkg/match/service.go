package match

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/Tuananh165-art/NexusChat/pkg/realtime"
)

type UserService interface {
	GetUserByID(ctx context.Context, uid uint64) (*User, error)
	GetUserIDBySession(ctx context.Context, sid string) (uint64, error)
	AddUserToChannel(ctx context.Context, channelID, userID uint64) error
}

type MatchingService interface {
	Match(ctx context.Context, userID uint64) (*MatchResult, error)
	BroadcastMatchResult(ctx context.Context, result *MatchResult) error
	RemoveUserFromWaitList(ctx context.Context, userID uint64) error
}

type UserServiceImpl struct {
	userRepo UserRepo
}

func NewUserServiceImpl(userRepo UserRepo) *UserServiceImpl {
	return &UserServiceImpl{userRepo}
}

func (svc *UserServiceImpl) GetUserByID(ctx context.Context, uid uint64) (*User, error) {
	user, err := svc.userRepo.GetUserByID(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("error get user %d: %w", uid, err)
	}
	return user, nil
}

func (svc *UserServiceImpl) GetUserIDBySession(ctx context.Context, sid string) (uint64, error) {
	userID, err := svc.userRepo.GetUserIDBySession(ctx, sid)
	if err != nil {
		return 0, fmt.Errorf("error get user id by sid %s: %w", sid, err)
	}
	return userID, nil
}

func (svc *UserServiceImpl) AddUserToChannel(ctx context.Context, channelID, userID uint64) error {
	if err := svc.userRepo.AddUserToChannel(ctx, channelID, userID); err != nil {
		return fmt.Errorf("error add user %d to channel %d: %w", userID, channelID, err)
	}
	return nil
}

type MatchingServiceImpl struct {
	matchRepo MatchingRepo
	chanRepo  ChannelRepo
}

func NewMatchingServiceImpl(matchRepo MatchingRepo, chanRepo ChannelRepo) *MatchingServiceImpl {
	return &MatchingServiceImpl{matchRepo, chanRepo}
}
func (svc *MatchingServiceImpl) Match(ctx context.Context, userID uint64) (*MatchResult, error) {
	matched, peerID, err := svc.matchRepo.PopOrPushWaitList(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("error match user %d: %w", userID, err)
	}
	if matched {
		if endpoint := os.Getenv("MATCH_GRPC_CLIENT_SAFETY_ENDPOINT"); endpoint != "" {
			filtered, filterErr := realtime.CallStructRPC(ctx, endpoint, "nexuschat.safety.v1.SafetyService", "BatchFilterCandidates", map[string]any{
				"user_id": strconv.FormatUint(userID, 10), "candidate_ids": strconv.FormatUint(peerID, 10),
			})
			if filterErr == nil && filtered != nil {
				values := filtered.GetFields()["candidate_ids"]
				if values != nil && len(values.GetListValue().GetValues()) == 0 {
					return &MatchResult{Matched: false}, nil
				}
			}
		}
		if endpoint := os.Getenv("MATCH_GRPC_CLIENT_DISCOVERY_ENDPOINT"); endpoint != "" {
			ranked, rankErr := realtime.CallStructRPC(ctx, endpoint, "nexuschat.discovery.v1.DiscoveryService", "RankCandidates", map[string]any{
				"user_id": strconv.FormatUint(userID, 10), "candidate_ids": strconv.FormatUint(peerID, 10),
			})
			if rankErr == nil && ranked != nil {
				values := ranked.GetFields()["candidates"]
				if values != nil && len(values.GetListValue().GetValues()) == 0 {
					return &MatchResult{Matched: false}, nil
				}
			}
		}
		newChannelID, accessToken, err := svc.chanRepo.CreateChannel(ctx)
		if err != nil {
			return nil, fmt.Errorf("error create channel: %w", err)
		}
		return &MatchResult{
			Matched:     true,
			UserID:      userID,
			PeerID:      peerID,
			ChannelID:   newChannelID,
			AccessToken: accessToken,
		}, nil
	}
	return &MatchResult{
		Matched: false,
	}, nil
}
func (svc *MatchingServiceImpl) BroadcastMatchResult(ctx context.Context, result *MatchResult) error {
	if err := svc.matchRepo.PublishMatchResult(ctx, result); err != nil {
		return fmt.Errorf("error broadcast match result: %w", err)
	}
	return nil
}
func (svc *MatchingServiceImpl) RemoveUserFromWaitList(ctx context.Context, userID uint64) error {
	if err := svc.matchRepo.RemoveFromWaitList(ctx, userID); err != nil {
		return fmt.Errorf("error remove user %d from wait list: %w", userID, err)
	}
	return nil
}
