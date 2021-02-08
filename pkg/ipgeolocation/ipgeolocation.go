package ipgeolocation

import (
	"encoding/csv"
	"net"
	"os"

	maxminddb "github.com/oschwald/maxminddb-golang"
)

var supportedCurrencies = map[string]string{
	"USD": "$",
	"EUR": "€",
	"GBP": "£",
}

type IPGeoLocation struct {
	c2c map[string]string
	db  *maxminddb.Reader
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

func NewIPGeoLocation(geoliteLocation, country2currencyLocation string) (IPGeoLocation, error) {
	r, err := maxminddb.Open(geoliteLocation)
	if err != nil {
		return IPGeoLocation{}, err
	}

	c2c, err := os.Open(country2currencyLocation)
	if err != nil {
		return IPGeoLocation{}, err
	}
	defer c2c.Close()
	c2cLines, err := csv.NewReader(c2c).ReadAll()
	if err != nil {
		return IPGeoLocation{}, err
	}

	c2cMap := make(map[string]string, len(c2cLines))
	for _, l := range c2cLines {
		c2cMap[l[0]] = l[1]
	}
	return IPGeoLocation{db: r, c2c: c2cMap}, nil
}

func (i IPGeoLocation) GetCurrencyForIP(ip string) (Currency, error) {
	ipNet := net.ParseIP(ip)
	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
	err := i.db.Lookup(ipNet, &record)
	if err != nil {
		return Currency{CurrencyUSD, "$"}, err
	}
	currencyCode := i.c2c[record.Country.ISOCode]
	if currencyCode == "" {
		return Currency{CurrencyUSD, "$"}, nil
	}
	if supportedCurrencies[currencyCode] == "" {
		return Currency{CurrencyUSD, "$"}, nil
	}
	return Currency{currencyCode, supportedCurrencies[currencyCode]}, nil
}

func (i IPGeoLocation) Close() {
	i.db.Close()
}
