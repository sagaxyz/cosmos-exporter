package main

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/simapp"
	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	ibcclienttypes "github.com/cosmos/ibc-go/v3/modules/core/02-client/types"
	ibcconnectiontypes "github.com/cosmos/ibc-go/v3/modules/core/03-connection/types"
	ibcchanneltypes "github.com/cosmos/ibc-go/v3/modules/core/04-channel/types"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

func ValidatorsHandler(w http.ResponseWriter, r *http.Request, grpcConn *grpc.ClientConn) {
	encCfg := simapp.MakeTestEncodingConfig()
	interfaceRegistry := encCfg.InterfaceRegistry

	requestStart := time.Now()

	sublogger := log.With().
		Str("request-id", uuid.New().String()).
		Logger()

	validatorsCommissionGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_validators_commission",
			Help:        "Commission of the Cosmos-based blockchain validator",
			ConstLabels: ConstLabels,
		},
		[]string{"address", "moniker"},
	)

	validatorsStatusGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_validators_status",
			Help:        "Status of the Cosmos-based blockchain validator",
			ConstLabels: ConstLabels,
		},
		[]string{"address", "moniker"},
	)

	validatorsJailedGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_validators_jailed",
			Help:        "Jailed status of the Cosmos-based blockchain validator",
			ConstLabels: ConstLabels,
		},
		[]string{"address", "moniker"},
	)

	validatorsTokensGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_validators_tokens",
			Help:        "Tokens of the Cosmos-based blockchain validator",
			ConstLabels: ConstLabels,
		},
		[]string{"address", "moniker", "denom"},
	)

	validatorsDelegatorSharesGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_validators_delegator_shares",
			Help:        "Delegator shares of the Cosmos-based blockchain validator",
			ConstLabels: ConstLabels,
		},
		[]string{"address", "moniker", "denom"},
	)

	validatorsMinSelfDelegationGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_validators_min_self_delegation",
			Help:        "Self declared minimum self delegation shares of the Cosmos-based blockchain validator",
			ConstLabels: ConstLabels,
		},
		[]string{"address", "moniker", "denom"},
	)

	validatorsMissedBlocksGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_validators_missed_blocks",
			Help:        "Missed blocks of the Cosmos-based blockchain validator",
			ConstLabels: ConstLabels,
		},
		[]string{"address", "moniker"},
	)

	validatorsRankGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_validators_rank",
			Help:        "Rank of the Cosmos-based blockchain validator",
			ConstLabels: ConstLabels,
		},
		[]string{"address", "moniker"},
	)

	validatorsIsActiveGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_validators_active",
			Help:        "1 if the Cosmos-based blockchain validator is in active set, 0 if no",
			ConstLabels: ConstLabels,
		},
		[]string{"address", "moniker"},
	)

	ibcChannelsGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_ibc_channels",
			Help:        "IBC channels opened by the validator",
			ConstLabels: ConstLabels,
		},
		[]string{"channel_id", "counterparty_channel_id", "status"},
	)

	ibcConnectionsGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_ibc_connections",
			Help:        "IBC connections opened by the validator",
			ConstLabels: ConstLabels,
		},
		[]string{"connection_id", "client_id", "counterparty_client_id", "counterparty_connection_id", "state"},
	)

	ibcClientsGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cosmos_ibc_clients",
			Help:        "IBC clients created by the validator",
			ConstLabels: ConstLabels,
		},
		[]string{"client_id", "status"},
	)

	registry := prometheus.NewRegistry()
	registry.MustRegister(validatorsCommissionGauge)
	registry.MustRegister(validatorsStatusGauge)
	registry.MustRegister(validatorsJailedGauge)
	registry.MustRegister(validatorsTokensGauge)
	registry.MustRegister(validatorsDelegatorSharesGauge)
	registry.MustRegister(validatorsMinSelfDelegationGauge)
	registry.MustRegister(validatorsMissedBlocksGauge)
	registry.MustRegister(validatorsRankGauge)
	registry.MustRegister(validatorsIsActiveGauge)
	registry.MustRegister(ibcChannelsGauge)
	registry.MustRegister(ibcConnectionsGauge)
	registry.MustRegister(ibcClientsGauge)

	var validators []stakingtypes.Validator
	var signingInfos []slashingtypes.ValidatorSigningInfo
	var validatorSetLength uint32

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying validators")
		queryStart := time.Now()

		stakingClient := stakingtypes.NewQueryClient(grpcConn)
		validatorsResponse, err := stakingClient.Validators(
			context.Background(),
			&stakingtypes.QueryValidatorsRequest{
				Pagination: &querytypes.PageRequest{
					Limit: Limit,
				},
			},
		)
		if err != nil {
			sublogger.Error().Err(err).Msg("Could not get validators")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying validators")
		validators = validatorsResponse.Validators

		// sorting by delegator shares to display rankings
		sort.Slice(validators, func(i, j int) bool {
			return validators[i].DelegatorShares.GT(validators[j].DelegatorShares)
		})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying validators signing infos")
		queryStart := time.Now()

		slashingClient := slashingtypes.NewQueryClient(grpcConn)
		signingInfosResponse, err := slashingClient.SigningInfos(
			context.Background(),
			&slashingtypes.QuerySigningInfosRequest{
				Pagination: &querytypes.PageRequest{
					Limit: Limit,
				},
			},
		)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get validators signing infos")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying validator signing infos")
		signingInfos = signingInfosResponse.Info
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying staking params")
		queryStart := time.Now()

		stakingClient := stakingtypes.NewQueryClient(grpcConn)
		paramsResponse, err := stakingClient.Params(
			context.Background(),
			&stakingtypes.QueryParamsRequest{},
		)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get staking params")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying staking params")
		validatorSetLength = paramsResponse.Params.MaxValidators
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying ibc channels")
		queryStart := time.Now()

		ibcClient := ibcchanneltypes.NewQueryClient(grpcConn)
		channelsResponse, err := ibcClient.Channels(
			context.Background(),
			&ibcchanneltypes.QueryChannelsRequest{
				Pagination: &querytypes.PageRequest{
					Limit: Limit,
				},
			},
		)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get IBC channels")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying IBC channels")

		for _, channel := range channelsResponse.Channels {
			ibcChannelsGauge.With(prometheus.Labels{
				"channel_id":              channel.ChannelId,
				"counterparty_channel_id": channel.Counterparty.ChannelId,
				"status":                  channel.State.String(),
			}).Set(1)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying ibc connections")
		queryStart := time.Now()

		ibcClient := ibcconnectiontypes.NewQueryClient(grpcConn)
		connectionsResponse, err := ibcClient.Connections(
			context.Background(),
			&ibcconnectiontypes.QueryConnectionsRequest{
				Pagination: &querytypes.PageRequest{
					Limit: Limit,
				},
			},
		)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get IBC connections")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying IBC connections")

		for _, connection := range connectionsResponse.Connections {
			ibcConnectionsGauge.With(prometheus.Labels{
				"connection_id":              connection.Id,
				"client_id":                  connection.ClientId,
				"counterparty_client_id":     connection.Counterparty.ClientId,
				"counterparty_connection_id": connection.Counterparty.ConnectionId,
				"state":                      connection.State.String(),
			}).Set(1)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying ibc clients")
		queryStart := time.Now()

		ibcClient := ibcclienttypes.NewQueryClient(grpcConn)
		clientsResponse, err := ibcClient.ClientStates(
			context.Background(),
			&ibcclienttypes.QueryClientStatesRequest{
				Pagination: &querytypes.PageRequest{
					Limit: Limit,
				},
			},
		)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get IBC clients")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying IBC clients")

		for _, client := range clientsResponse.ClientStates {
			clientStatusResponse, err := ibcClient.ClientStatus(
				context.Background(),
				&ibcclienttypes.QueryClientStatusRequest{
					ClientId: client.ClientId,
				},
			)
			if err != nil {
				sublogger.Error().
					Err(err).
					Msg("Could not get IBC client state")
				return
			}

			ibcClientsGauge.With(prometheus.Labels{
				"client_id": client.ClientId,
				"status":    clientStatusResponse.Status,
			}).Set(1)
		}
	}()

	wg.Wait()

	sublogger.Debug().
		Int("signingLength", len(signingInfos)).
		Int("validatorsLength", len(validators)).
		Msg("Validators info")

	for index, validator := range validators {
		// because cosmos's dec doesn't have .toFloat64() method or whatever and returns everything as int
		rate, err := strconv.ParseFloat(validator.Commission.CommissionRates.Rate.String(), 64)
		if err != nil {
			log.Error().
				Err(err).
				Str("address", validator.OperatorAddress).
				Msg("Could not get commission")
		} else {
			validatorsCommissionGauge.With(prometheus.Labels{
				"address": validator.OperatorAddress,
				"moniker": validator.Description.Moniker,
			}).Set(rate)
		}

		validatorsStatusGauge.With(prometheus.Labels{
			"address": validator.OperatorAddress,
			"moniker": validator.Description.Moniker,
		}).Set(float64(validator.Status))

		// golang doesn't have a ternary operator, so we have to stick with this ugly solution
		var jailed float64

		if validator.Jailed {
			jailed = 1
		} else {
			jailed = 0
		}
		validatorsJailedGauge.With(prometheus.Labels{
			"address": validator.OperatorAddress,
			"moniker": validator.Description.Moniker,
		}).Set(jailed)

		// because cosmos's dec doesn't have .toFloat64() method or whatever and returns everything as int
		if value, err := strconv.ParseFloat(validator.Tokens.String(), 64); err != nil {
			sublogger.Error().
				Str("address", validator.OperatorAddress).
				Err(err).
				Msg("Could not parse delegator tokens")
		} else {
			validatorsTokensGauge.With(prometheus.Labels{
				"address": validator.OperatorAddress,
				"moniker": validator.Description.Moniker,
				"denom":   Denom,
			}).Set(value / DenomCoefficient) // a better way to do this is using math/big Div then checking IsInt64
		}

		// because cosmos's dec doesn't have .toFloat64() method or whatever and returns everything as int
		if value, err := strconv.ParseFloat(validator.DelegatorShares.String(), 64); err != nil {
			sublogger.Error().
				Str("address", validator.OperatorAddress).
				Err(err).
				Msg("Could not parse delegator shares")
		} else {
			validatorsDelegatorSharesGauge.With(prometheus.Labels{
				"address": validator.OperatorAddress,
				"moniker": validator.Description.Moniker,
				"denom":   Denom,
			}).Set(value / DenomCoefficient)
		}

		// because cosmos's dec doesn't have .toFloat64() method or whatever and returns everything as int
		if value, err := strconv.ParseFloat(validator.MinSelfDelegation.String(), 64); err != nil {
			sublogger.Error().
				Str("address", validator.OperatorAddress).
				Err(err).
				Msg("Could not parse validator min self delegation")
		} else {
			validatorsMinSelfDelegationGauge.With(prometheus.Labels{
				"address": validator.OperatorAddress,
				"moniker": validator.Description.Moniker,
				"denom":   Denom,
			}).Set(value / DenomCoefficient)
		}

		err = validator.UnpackInterfaces(interfaceRegistry) // Unpack interfaces, to populate the Anys' cached values
		if err != nil {
			sublogger.Error().
				Str("address", validator.OperatorAddress).
				Err(err).
				Msg("Could not get unpack validator inferfaces")
		}

		pubKey, err := validator.GetConsAddr()
		if err != nil {
			sublogger.Error().
				Str("address", validator.OperatorAddress).
				Err(err).
				Msg("Could not get validator pubkey")
		}

		var signingInfo slashingtypes.ValidatorSigningInfo
		found := false

		for _, signingInfoIterated := range signingInfos {
			if pubKey.String() == signingInfoIterated.Address {
				found = true
				signingInfo = signingInfoIterated
				break
			}
		}

		if !found {
			sublogger.Debug().
				Str("address", validator.OperatorAddress).
				Msg("Could not get signing info for validator")
			continue
		}

		if validator.Status == stakingtypes.Bonded {
			validatorsMissedBlocksGauge.With(prometheus.Labels{
				"address": validator.OperatorAddress,
				"moniker": validator.Description.Moniker,
			}).Set(float64(signingInfo.MissedBlocksCounter))
		} else {
			sublogger.Trace().
				Str("address", validator.OperatorAddress).
				Msg("Validator is not active, not returning missed blocks amount.")
		}

		validatorsRankGauge.With(prometheus.Labels{
			"address": validator.OperatorAddress,
			"moniker": validator.Description.Moniker,
		}).Set(float64(index + 1))

		if validatorSetLength != 0 {
			// golang doesn't have a ternary operator, so we have to stick with this ugly solution
			var active float64

			if index+1 <= int(validatorSetLength) {
				active = 1
			} else {
				active = 0
			}

			validatorsIsActiveGauge.With(prometheus.Labels{
				"address": validator.OperatorAddress,
				"moniker": validator.Description.Moniker,
			}).Set(active)
		}
	}

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
