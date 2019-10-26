package ipgeolocation

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

type IPGeoLocation struct {
	apiKey string
	uri    string
}

type Currency struct {
	Code   string
	Symbol string
}

const (
	CurrencyUSD = "USD"
	CurrencyEUR = "EUR"
	CurrencyGBP = "GBP"
)

func NewIPGeoLocation(apiKey, URI string) IPGeoLocation {
	return IPGeoLocation{apiKey: apiKey, uri: URI}
}

func (i IPGeoLocation) GetCurrencyForIP(ip string) (Currency, error) {
	res, err := http.Get(fmt.Sprintf("%s?apiKey=%s&ip=%s", i.uri, i.apiKey, ip))
	if err != nil {
		return Currency{CurrencyUSD, "$"}, errors.Wrapf(err, "unable to call api.ipgeolocation.io for IP lookup %#v", ip[0])
	}
	defer res.Body.Close()
	var meta map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&meta)
	if err != nil {
		return Currency{CurrencyUSD, "$"}, errors.Wrapf(err, "unable to parse api.ipgeolocation.io response for IP lookup %#v", ip)
	}
	currencyMeta, ok := meta["currency"].(map[string]interface{})
	if !ok {
		return Currency{CurrencyUSD, "$"}, fmt.Errorf("unable to cast currency from APIs: %+v", currencyMeta["currency"])
	}
	currencyCode, ok := currencyMeta["code"].(string)
	if !ok {
		return Currency{CurrencyUSD, "$"}, fmt.Errorf("unable to cast code from APIs: %+v", currencyMeta["code"])
	}
	currencySymbol, ok := currencyMeta["symbol"].(string)
	if !ok {
		return Currency{CurrencyUSD, "$"}, fmt.Errorf("unable to cast symbol from APIs: %+v", currencyMeta["symbol"])
	}
	// default to USD in case currencyCode is neither EUR / GBP / USD
	if currencyCode != CurrencyEUR && currencyCode != CurrencyGBP && currencyCode != CurrencyUSD {
		return Currency{CurrencyUSD, "$"}, nil
	}
	return Currency{currencyCode, currencySymbol}, nil
}
