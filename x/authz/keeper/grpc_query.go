package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	proto "github.com/gogo/protobuf/proto"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/cosmos-sdk/x/authz"
)

var _ authz.QueryServer = Keeper{}

// Authorizations implements the Query/Grants gRPC method.
func (k Keeper) Grants(c context.Context, req *authz.QueryGrantsRequest) (*authz.QueryGrantsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	granter, err := sdk.AccAddressFromBech32(req.Granter)
	if err != nil {
		return nil, err
	}

	grantee, err := sdk.AccAddressFromBech32(req.Grantee)
	if err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(k.storeKey)
	key := grantStoreKey(grantee, granter, "")
	authStore := prefix.NewStore(store, key)

	if req.MsgTypeUrl != "" {
		authorization, expiration := k.GetCleanAuthorization(ctx, grantee, granter, req.MsgTypeUrl)
		if authorization == nil {
			return nil, status.Errorf(codes.NotFound, "no authorization found for %s type", req.MsgTypeUrl)
		}
		authorizationAny, err := codectypes.NewAnyWithValue(authorization)
		if err != nil {
			return nil, status.Errorf(codes.Internal, err.Error())
		}
		return &authz.QueryGrantsResponse{
			Grants: []*authz.Grant{{
				Authorization: authorizationAny,
				Expiration:    expiration,
			}},
		}, nil
	}

	var authorizations []*authz.Grant
	pageRes, err := query.FilteredPaginate(authStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
		auth, err := unmarshalAuthorization(k.cdc, value)
		if err != nil {
			return false, err
		}
		auth1 := auth.GetAuthorization()
		if accumulate {
			msg, ok := auth1.(proto.Message)
			if !ok {
				return false, status.Errorf(codes.Internal, "can't protomarshal %T", msg)
			}

			authorizationAny, err := codectypes.NewAnyWithValue(msg)
			if err != nil {
				return false, status.Errorf(codes.Internal, err.Error())
			}
			authorizations = append(authorizations, &authz.Grant{
				Authorization: authorizationAny,
				Expiration:    auth.Expiration,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return &authz.QueryGrantsResponse{
		Grants:     authorizations,
		Pagination: pageRes,
	}, nil
}

// IssuedGrants implements the Query/IssuedGrants gRPC method.
func (k Keeper) IssuedGrants(c context.Context, req *authz.QueryIssuedGrantsRequest) (*authz.QueryIssuedGrantsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	granter, err := sdk.AccAddressFromBech32(req.Granter)
	if err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(c)
	store := ctx.KVStore(k.storeKey)
	authzStore := prefix.NewStore(store, grantStoreKey(nil, granter, ""))

	var grants []*authz.GrantAuthorization
	pageRes, err := query.FilteredPaginate(authzStore, req.Pagination, func(key []byte, value []byte,
		accumulate bool) (bool, error) {
		auth, err := unmarshalAuthorization(k.cdc, value)
		if err != nil {
			return false, err
		}

		auth1 := auth.GetAuthorization()
		if accumulate {
			any, err := codectypes.NewAnyWithValue(auth1)
			if err != nil {
				return false, status.Errorf(codes.Internal, err.Error())
			}

			grantee, granter := addressesFromGrantStoreKey(key)

			grants = append(grants, &authz.GrantAuthorization{
				Authorization: any,
				Expiration:    auth.Expiration,
				Granter:       granter.String(),
				Grantee:       grantee.String(),
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return &authz.QueryIssuedGrantsResponse{
		Grants:     grants,
		Pagination: pageRes,
	}, nil
}

// ReceivedGrants implements the Query/ReceivedGrants gRPC method.
func (k Keeper) ReceivedGrants(c context.Context, req *authz.QueryReceivedGrantsRequest) (*authz.QueryReceivedGrantsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	grantee, err := sdk.AccAddressFromBech32(req.Grantee)
	if err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(c)
	store := ctx.KVStore(k.storeKey)
	authzStore := prefix.NewStore(store, grantStoreKey(grantee, nil, ""))

	var authorizations []*authz.GrantAuthorization
	pageRes, err := query.FilteredPaginate(authzStore, req.Pagination, func(key []byte, value []byte,
		accumulate bool) (bool, error) {
		auth, err := unmarshalAuthorization(k.cdc, value)
		if err != nil {
			return false, err
		}

		auth1 := auth.GetAuthorization()
		if accumulate {
			any, err := codectypes.NewAnyWithValue(auth1)
			if err != nil {
				return false, status.Errorf(codes.Internal, err.Error())
			}

			grantee, granter := addressesFromGrantStoreKey(key)

			authorizations = append(authorizations, &authz.GrantAuthorization{
				Authorization: any,
				Expiration:    auth.Expiration,
				Granter:       granter.String(),
				Grantee:       grantee.String(),
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return &authz.QueryReceivedGrantsResponse{
		Grants:     authorizations,
		Pagination: pageRes,
	}, nil
}

// unmarshal an authorization from a store value
func unmarshalAuthorization(cdc codec.BinaryCodec, value []byte) (v authz.Grant, err error) {
	err = cdc.Unmarshal(value, &v)
	return v, err
}
