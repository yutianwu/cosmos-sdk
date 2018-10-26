package simulation

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/mock/simulation"
	abci "github.com/tendermint/tendermint/abci/types"
)

// AllInvariants runs all invariants of the distribution module
// Currently: total supply, positive power
func AllInvariants(d distr.Keeper, sk distr.StakeKeeper) simulation.Invariant {

	return func(app *baseapp.BaseApp, header abci.Header) error {
		err := ValAccumInvariants(d, sk)(app, header)
		if err != nil {
			return err
		}
		return nil
	}
}

// ValAccumInvariants checks that the fee pool accum == sum all validators' accum
func ValAccumInvariants(k distr.Keeper, sk distr.StakeKeeper) simulation.Invariant {

	return func(app *baseapp.BaseApp, header abci.Header) error {
		ctx := app.NewContext(false, header)
		height := ctx.BlockHeight()

		valAccum := sdk.ZeroDec()
		k.IterateValidatorDistInfos(ctx, func(_ int64, vdi distr.ValidatorDistInfo) bool {
			lastValPower := sk.GetLastValidatorPower(ctx, vdi.OperatorAddr)
			valAccum = valAccum.Add(vdi.GetValAccum(height, lastValPower))

			addr, _ := sdk.ValAddressFromHex("93FDA0FC051FF6F726094DE8ABEB724433E63106")
			if vdi.OperatorAddr.Equals(addr) {
				fmt.Printf("\ndebug vdi: %v ", vdi)
				validator := sk.Validator(ctx, vdi.OperatorAddr)
				fmt.Printf("Jailed: %v ", validator.GetJailed())
				fmt.Printf("GetStatus: %v ", validator.GetStatus())
			}
			return false
		})

		lastTotalPower := sdk.NewDecFromInt(sk.GetLastTotalPower(ctx))
		feePool := k.GetFeePool(ctx)
		totalAccum := feePool.GetTotalValAccum(height, lastTotalPower)

		if !totalAccum.Equal(valAccum) {
			//k.IterateValidatorDistInfos(ctx, func(_ int64, vdi distr.ValidatorDistInfo) bool {
			//lastValPower := sk.GetLastValidatorPower(ctx, vdi.OperatorAddr)
			//valAccum = vdi.GetValAccum(height, lastValPower)
			//if valAccum.GT(sdk.ZeroDec()) {
			//fmt.Printf("debug vdi: %v\n", vdi)
			//validator := sk.Validator(ctx, vdi.OperatorAddr)
			//fmt.Printf("debug validator.Jailed: %v\n", validator.GetJailed())
			//fmt.Printf("debug validator.GetStatus: %v\n", validator.GetStatus())
			//}
			//return false
			//})
			return fmt.Errorf("validator accum invariance: \n\tfee pool totalAccum: %v"+
				"\n\tvalidator accum \t%v\n"+
				"totalAccum %v\n last total power %v\n"+
				"height %v\n, f.TotalValAccum %v\n",
				totalAccum.String(), valAccum.String(),
				totalAccum, lastTotalPower, height, feePool.TotalValAccum)
		}

		return nil
	}
}
