#!/bin/bash 
__dir=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source $__dir/useful_commands.sh
. ${__dir}/vars/variables.sh
# Making sure old screens are not running
killall screen
screen -wipe
GASPRICE="0.000000001ulava"

lavad tx gov submit-proposal spec-add ./cookbook/specs/spec_add_ibc.json,./cookbook/specs/spec_add_cosmoswasm.json,./cookbook/specs/spec_add_cosmossdk.json,./cookbook/specs/spec_add_cosmossdk_full.json,./cookbook/specs/spec_add_ethereum.json,./cookbook/specs/spec_add_cosmoshub.json,./cookbook/specs/spec_add_lava.json,./cookbook/specs/spec_add_osmosis.json,./cookbook/specs/spec_add_fantom.json,./cookbook/specs/spec_add_celo.json,./cookbook/specs/spec_add_optimism.json,./cookbook/specs/spec_add_arbitrum.json,./cookbook/specs/spec_add_starknet.json,./cookbook/specs/spec_add_aptos.json,./cookbook/specs/spec_add_juno.json,./cookbook/specs/spec_add_polygon.json,./cookbook/specs/spec_add_evmos.json,./cookbook/specs/spec_add_base.json,./cookbook/specs/spec_add_canto.json,./cookbook/specs/spec_add_sui.json,./cookbook/specs/spec_add_solana.json,./cookbook/specs/spec_add_bsc.json,./cookbook/specs/spec_add_axelar.json -y --from alice --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx gov vote 1 yes -y --from alice --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE

lavad tx gov submit-proposal plans-add ./cookbook/plans/default.json -y --from alice --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx gov vote 2 yes -y --from alice --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE

CLIENTSTAKE="500000000000ulava"
PROVIDERSTAKE="500000000000ulava"

PROVIDER1_LISTENER="127.0.0.1:2221"
PROVIDER2_LISTENER="127.0.0.1:2222"
PROVIDER3_LISTENER="127.0.0.1:2223"

sleep 4
lavad tx pairing stake-client "ETH1"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "GTH1"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "COS3"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "FTM250" $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "CELO"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "LAV1"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "COS4"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto"  --gas-prices $GASPRICE
lavad tx pairing stake-client "ALFAJORES" $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto"  --gas-prices $GASPRICE
lavad tx pairing stake-client "ARB1"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto"  --gas-prices $GASPRICE
lavad tx pairing stake-client "ARBN"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto"  --gas-prices $GASPRICE
lavad tx pairing stake-client "APT1"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto"  --gas-prices $GASPRICE
lavad tx pairing stake-client "STRK"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto"  --gas-prices $GASPRICE
lavad tx pairing stake-client "JUN1"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "COS5"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "POLYGON1" $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "EVMOS"  $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "OPTM"   $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "BASET"  $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "CANTO"  $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "SUIT"  $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "SOLANA"  $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "BSC"  $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing stake-client "AXELAR"  $CLIENTSTAKE 1 -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE

CHAINS="ETH1,GTH1,COS3,FTM250,CELO,LAV1,COS4,ALFAJORES,ARB1,ARBN,APT1,STRK,JUN1,COS5,POLYGON1,EVMOS,OPTM,BASET,CANTO,SUIT,SOLANA,BSC,AXELAR"
# stake providers on all chains
lavad tx pairing bulk-stake-provider $CHAINS $PROVIDERSTAKE "$PROVIDER1_LISTENER,1" 1 -y --from servicer1 --provider-moniker "dummyMoniker" --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing bulk-stake-provider $CHAINS $PROVIDERSTAKE "$PROVIDER2_LISTENER,1" 1 -y --from servicer2 --provider-moniker "dummyMoniker" --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
lavad tx pairing bulk-stake-provider $CHAINS $PROVIDERSTAKE "$PROVIDER3_LISTENER,1" 1 -y --from servicer3 --provider-moniker "dummyMoniker" --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE

echo "---------------Queries------------------"
lavad query pairing providers "ETH1"
lavad query pairing clients "ETH1"

# we need to wait for the next epoch for the stake to take action.
sleep_until_next_epoch

. ${__dir}/setup_providers.sh

