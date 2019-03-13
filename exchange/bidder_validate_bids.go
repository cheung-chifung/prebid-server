package exchange

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mxmCherry/openrtb"
	"github.com/prebid/prebid-server/currencies"
	"github.com/prebid/prebid-server/openrtb_ext"
	"golang.org/x/text/currency"
)

// ensureValidBids returns a bidder that removes invalid bids from the argument bidder's response.
// These will be converted into errors instead.
//
// The goal here is to make sure that the response contains Bids which are valid given the initial Request,
// so that Publishers can trust the Bids they get from Prebid Server.
func ensureValidBids(bidder AdaptedBidder) AdaptedBidder {
	return &validatedBidder{
		bidder: bidder,
	}
}

type validatedBidder struct {
	bidder AdaptedBidder
}

func (v *validatedBidder) RequestBid(ctx context.Context, request *openrtb.BidRequest, name openrtb_ext.BidderName, bidAdjustment float64, conversions currencies.Conversions) (*PBSOrtbSeatBid, []error) {
	seatBid, errs := v.bidder.RequestBid(ctx, request, name, bidAdjustment, conversions)
	if validationErrors := removeInvalidBids(request, seatBid); len(validationErrors) > 0 {
		errs = append(errs, validationErrors...)
	}
	return seatBid, errs
}

// validateBids will run some validation checks on the returned bids and excise any invalid bids
func removeInvalidBids(request *openrtb.BidRequest, seatBid *PBSOrtbSeatBid) []error {
	// Exit early if there is nothing to do.
	if seatBid == nil || len(seatBid.Bids) == 0 {
		return nil
	}

	// By design, default currency is USD.
	if cerr := validateCurrency(request.Cur, seatBid.Currency); cerr != nil {
		seatBid.Bids = nil
		return []error{cerr}
	}

	errs := make([]error, 0, len(seatBid.Bids))
	validBids := make([]*PBSOrtbBid, 0, len(seatBid.Bids))
	for _, bid := range seatBid.Bids {
		if ok, berr := validateBid(bid); ok {
			validBids = append(validBids, bid)
		} else {
			errs = append(errs, berr)
		}
	}
	seatBid.Bids = validBids
	return errs
}

// validateCurrency will run currency validation checks and return true if it passes, false otherwise.
func validateCurrency(requestAllowedCurrencies []string, bidCurrency string) error {
	// Default currency is `USD` by design.
	defaultCurrency := "USD"
	// Make sure bid currency is a valid ISO currency code
	if bidCurrency == "" {
		// If bid currency is not set, then consider it's default currency.
		bidCurrency = defaultCurrency
	}
	currencyUnit, cerr := currency.ParseISO(bidCurrency)
	if cerr != nil {
		return cerr
	}
	// Make sure the bid currency is allowed from bid request via `cur` field.
	// If `cur` field array from bid request is empty, then consider it accepts the default currency.
	currencyAllowed := false
	if len(requestAllowedCurrencies) == 0 {
		requestAllowedCurrencies = []string{defaultCurrency}
	}
	for _, allowedCurrency := range requestAllowedCurrencies {
		if strings.ToUpper(allowedCurrency) == currencyUnit.String() {
			currencyAllowed = true
			break
		}
	}
	if currencyAllowed == false {
		return fmt.Errorf(
			"Bid currency is not allowed. Was '%s', wants: ['%s']",
			currencyUnit.String(),
			strings.Join(requestAllowedCurrencies, "', '"),
		)
	}

	return nil
}

// validateBid will run the supplied bid through validation checks and return true if it passes, false otherwise.
func validateBid(bid *PBSOrtbBid) (bool, error) {
	if bid.Bid == nil {
		return false, errors.New("Empty bid object submitted.")
	}

	if bid.Bid.ID == "" {
		return false, errors.New("Bid missing required field 'id'")
	}
	if bid.Bid.ImpID == "" {
		return false, fmt.Errorf("Bid \"%s\" missing required field 'impid'", bid.Bid.ID)
	}
	if bid.Bid.Price <= 0.0 {
		return false, fmt.Errorf("Bid \"%s\" does not contain a positive 'price'", bid.Bid.ID)
	}
	if bid.Bid.CrID == "" {
		return false, fmt.Errorf("Bid \"%s\" missing creative ID", bid.Bid.ID)
	}

	return true, nil
}
