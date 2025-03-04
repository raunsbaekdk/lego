package certificate

import (
	"time"

	"github.com/go-acme/lego/v4/acme"
	"github.com/go-acme/lego/v4/log"
)

const (
	// overallRequestLimit is the overall number of request per second
	// limited on the "new-reg", "new-authz" and "new-cert" endpoints.
	// From the documentation the limitation is 20 requests per second,
	// but using 20 as value doesn't work but 18 do.
	overallRequestLimit = 18
)

func (c *Certifier) getAuthorizations(order acme.ExtendedOrder) ([]acme.Authorization, error) {
	resc, errc := make(chan acme.Authorization), make(chan domainError)

	requestLimit := c.options.OverallRequestLimit
	if c.options.OverallRequestLimit <= 0 {
		requestLimit = overallRequestLimit
	}

	delay := time.Second / requestLimit

	log.Infof("Delay %s / OverallRequestLimit %d / overallRequestLimit %d / requestLimit %d", delay, c.options.OverallRequestLimit, overallRequestLimit, requestLimit)

	for _, authzURL := range order.Authorizations {
		time.Sleep(delay)

		go func(authzURL string) {
			authz, err := c.core.Authorizations.Get(authzURL)
			if err != nil {
				errc <- domainError{Domain: authz.Identifier.Value, Error: err}
				return
			}

			resc <- authz
		}(authzURL)
	}

	var responses []acme.Authorization
	failures := make(obtainError)
	for i := 0; i < len(order.Authorizations); i++ {
		select {
		case res := <-resc:
			responses = append(responses, res)
		case err := <-errc:
			failures[err.Domain] = err.Error
		}
	}

	for i, auth := range order.Authorizations {
		log.Infof("[%s] AuthURL: %s", order.Identifiers[i].Value, auth)
	}

	close(resc)
	close(errc)

	// be careful to not return an empty failures map;
	// even if empty, they become non-nil error values
	if len(failures) > 0 {
		return responses, failures
	}
	return responses, nil
}

func (c *Certifier) deactivateAuthorizations(order acme.ExtendedOrder, force bool) {
	for _, authzURL := range order.Authorizations {
		auth, err := c.core.Authorizations.Get(authzURL)
		if err != nil {
			log.Infof("Unable to get the authorization for: %s", authzURL)
			continue
		}

		if auth.Status == acme.StatusValid && !force {
			log.Infof("Skipping deactivating of valid auth: %s", authzURL)
			continue
		}

		log.Infof("Deactivating auth: %s", authzURL)
		if c.core.Authorizations.Deactivate(authzURL) != nil {
			log.Infof("Unable to deactivate the authorization: %s", authzURL)
		}
	}
}
