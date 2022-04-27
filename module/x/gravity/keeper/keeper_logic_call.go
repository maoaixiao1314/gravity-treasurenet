package keeper

import (
	"encoding/hex"
	"fmt"

	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/Gravity-Bridge/Gravity-Bridge/module/x/gravity/types"
)

/////////////////////////////
//       LOGICCALLS        //
/////////////////////////////

// GetOutgoingLogicCall gets an outgoing logic call
func (k Keeper) GetOutgoingLogicCall(ctx sdk.Context, invalidationID []byte, invalidationNonce uint64) *types.OutgoingLogicCall {
	store := ctx.KVStore(k.storeKey)
	call := types.OutgoingLogicCall{
		Transfers:            []types.ERC20Token{}, //这些是在执行之前发送到逻辑合约的代币。然后合约可以使用代币采取行动。例如，Gravity 可以向逻辑合约发送一些 Uniswap LP 代币，然后它将用于从 Uniswap 赎回流动性。
		Fees:                 []types.ERC20Token{}, //这些代币将由核心 Gravity.sol 合约支付给 Gravity 中继器以执行逻辑调用。费用在逻辑合约执行后支付，因此可以将逻辑合约执行后收到的代币支付给中继者，然后发送回核心 Gravity 合约
		LogicContractAddress: "",                   //这是核心 Gravity 合约调用以执行任意逻辑的逻辑合约的地址。注意：这可能是实际的逻辑合约，也可能是多次调用逻辑合约的批处理合约。/solidity/test文件夹中的示例。
		Payload:              []byte{},             //这是将在逻辑合约上执行的以太坊 abi 编码函数调用。如果您使用的是批处理中间件合约，则此 abi 编码函数调用本身将包含实际逻辑合约上的 abi 编码函数调用数组
		Timeout:              0,
		InvalidationId:       invalidationID,
		InvalidationNonce:    invalidationNonce, //invalidation_id并invalidation_nonce在 Gravity 任意逻辑调用功能中用作重放保护
		Block:                0,
	}
	k.cdc.MustUnmarshal(store.Get(types.GetOutgoingLogicCallKey(invalidationID, invalidationNonce)), &call)
	return &call
}

// SetOutogingLogicCall sets an outgoing logic call, panics if one already exists at this
// index, since we collect signatures over logic calls no mutation can be valid
func (k Keeper) SetOutgoingLogicCall(ctx sdk.Context, call types.OutgoingLogicCall) {
	store := ctx.KVStore(k.storeKey)

	// Store checkpoint to prove that this logic call actually happened
	checkpoint := call.GetCheckpoint(k.GetGravityID(ctx))
	k.SetPastEthSignatureCheckpoint(ctx, checkpoint)
	key := types.GetOutgoingLogicCallKey(call.InvalidationId, call.InvalidationNonce)
	if store.Has(key) {
		panic("Can not overwrite logic call")
	}
	store.Set(key,
		k.cdc.MustMarshal(&call))
}

// DeleteOutgoingLogicCall deletes outgoing logic calls
func (k Keeper) DeleteOutgoingLogicCall(ctx sdk.Context, invalidationID []byte, invalidationNonce uint64) {
	ctx.KVStore(k.storeKey).Delete(types.GetOutgoingLogicCallKey(invalidationID, invalidationNonce))
}

// IterateOutgoingLogicCalls iterates over outgoing logic calls
func (k Keeper) IterateOutgoingLogicCalls(ctx sdk.Context, cb func([]byte, types.OutgoingLogicCall) bool) {
	prefixStore := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyOutgoingLogicCall)
	iter := prefixStore.Iterator(nil, nil)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var call types.OutgoingLogicCall
		k.cdc.MustUnmarshal(iter.Value(), &call)
		// cb returns true to stop early
		if cb(iter.Key(), call) {
			break
		}
	}
}

// GetOutgoingLogicCalls returns the outgoing logic calls
func (k Keeper) GetOutgoingLogicCalls(ctx sdk.Context) (out []types.OutgoingLogicCall) {
	k.IterateOutgoingLogicCalls(ctx, func(_ []byte, call types.OutgoingLogicCall) bool {
		out = append(out, call)
		return false
	})
	return
}

// CancelOutgoingLogicCalls releases all TX in the batch and deletes the batch
func (k Keeper) CancelOutgoingLogicCall(ctx sdk.Context, invalidationId []byte, invalidationNonce uint64) error {
	call := k.GetOutgoingLogicCall(ctx, invalidationId, invalidationNonce)
	if call == nil {
		return types.ErrUnknown
	}
	// Delete batch since it is finished
	k.DeleteOutgoingLogicCall(ctx, call.InvalidationId, call.InvalidationNonce)

	// a consuming application will have to watch for this event and act on it
	ctx.EventManager().EmitTypedEvent(
		&types.EventOutgoingLogicCallCanceled{
			LogicCallInvalidationId:    fmt.Sprint(call.InvalidationId),
			LogicCallInvalidationNonce: fmt.Sprint(call.InvalidationNonce),
		},
	)

	return nil
}

/////////////////////////////
//       LOGICCONFIRMS     //
/////////////////////////////

// SetLogicCallConfirm sets a logic confirm in the store
func (k Keeper) SetLogicCallConfirm(ctx sdk.Context, msg *types.MsgConfirmLogicCall) {
	bytes, err := hex.DecodeString(msg.InvalidationId)
	if err != nil {
		panic(err)
	}

	acc, err := sdk.AccAddressFromBech32(msg.Orchestrator)
	if err != nil {
		panic(err)
	}

	ctx.KVStore(k.storeKey).
		Set(types.GetLogicConfirmKey(bytes, msg.InvalidationNonce, acc), k.cdc.MustMarshal(msg))
}

// GetLogicCallConfirm gets a logic confirm from the store
func (k Keeper) GetLogicCallConfirm(ctx sdk.Context, invalidationId []byte, invalidationNonce uint64, val sdk.AccAddress) *types.MsgConfirmLogicCall {
	if err := sdk.VerifyAddressFormat(val); err != nil {
		ctx.Logger().Error("invalid val address")
		return nil
	}
	store := ctx.KVStore(k.storeKey)
	data := store.Get(types.GetLogicConfirmKey(invalidationId, invalidationNonce, val))
	if data == nil {
		return nil
	}
	out := types.MsgConfirmLogicCall{
		InvalidationId:    "",
		InvalidationNonce: invalidationNonce,
		EthSigner:         "",
		Orchestrator:      "",
		Signature:         "",
	}
	k.cdc.MustUnmarshal(data, &out)
	return &out
}

// DeleteLogicCallConfirm deletes a logic confirm from the store
func (k Keeper) DeleteLogicCallConfirm(
	ctx sdk.Context,
	invalidationID []byte,
	invalidationNonce uint64,
	val sdk.AccAddress) {
	ctx.KVStore(k.storeKey).Delete(types.GetLogicConfirmKey(invalidationID, invalidationNonce, val))
}

// IterateLogicConfirmByInvalidationIDAndNonce iterates over all logic confirms stored by nonce
func (k Keeper) IterateLogicConfirmByInvalidationIDAndNonce(
	ctx sdk.Context,
	invalidationID []byte,
	invalidationNonce uint64,
	cb func([]byte, *types.MsgConfirmLogicCall) bool) {
	store := ctx.KVStore(k.storeKey)
	prefix := types.GetLogicConfirmNonceInvalidationIdPrefix(invalidationID, invalidationNonce)
	iter := store.Iterator(prefixRange([]byte(prefix)))

	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		confirm := types.MsgConfirmLogicCall{
			InvalidationId:    "",
			InvalidationNonce: invalidationNonce,
			EthSigner:         "",
			Orchestrator:      "",
			Signature:         "",
		}
		k.cdc.MustUnmarshal(iter.Value(), &confirm)
		// cb returns true to stop early
		if cb(iter.Key(), &confirm) {
			break
		}
	}
}

// GetLogicConfirmsByInvalidationIdAndNonce returns the logic call confirms
func (k Keeper) GetLogicConfirmByInvalidationIDAndNonce(ctx sdk.Context, invalidationId []byte, invalidationNonce uint64) (out []types.MsgConfirmLogicCall) {
	k.IterateLogicConfirmByInvalidationIDAndNonce(ctx, invalidationId, invalidationNonce, func(_ []byte, msg *types.MsgConfirmLogicCall) bool {
		out = append(out, *msg)
		return false
	})
	return
}
