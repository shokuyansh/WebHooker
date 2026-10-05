package main

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"github.com/shokuyansh/Webhooker/internal/data"
	"github.com/shokuyansh/Webhooker/internal/urlguard"
)

func (app *application) listAllDeliveries(w http.ResponseWriter, r *http.Request) {
	deliveries, err := app.models.Deliveries.ListAll()
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	err = app.writeJSON(w, envelope{"deliveries": deliveries}, http.StatusOK, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) performDelivery(delivery *data.Delivery, ctx context.Context) error {
	event, err := app.models.Events.Get(delivery.EventID)
	if err != nil {
		app.logger.Error(err.Error())
		return err
	}

	webhook, err := app.models.Webhooks.Get(int(event.ProjectID), int(delivery.WebHookID))
	if err != nil {
		app.logger.Error(err.Error())
		return err
	}

	now := time.Now().UTC()
	delivery.LastAttemptAt = &now

	timeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(timeCtx, http.MethodPost, webhook.CallbackURL, bytes.NewBuffer(event.Payload))
	if err != nil {
		app.logger.Error(err.Error())
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: urlguard.Dialer(2 * time.Second).DialContext,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		app.logger.Error(err.Error())
		delivery.ResponseStatus = nil
		delivery.Status = "FAILED"
	} else {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			delivery.Status = "SUCCESS"
			delivery.ResponseStatus = &resp.StatusCode
		} else {
			delivery.Status = "FAILED"
			delivery.ResponseStatus = &resp.StatusCode
		}
		resp.Body.Close()
	}
	err = app.models.Deliveries.Update(delivery)
	if err != nil {
		app.logger.Error(err.Error())
		return err
	}
	return nil
}

func (app *application) processPendingDeliveries(ctx context.Context) error {
	pendingDeliveries, err := app.models.Deliveries.PendingDeliveries()
	if err != nil {
		app.logger.Error(err.Error())
		return err
	}
	for _, delivery := range pendingDeliveries {
		err := app.performDelivery(delivery, ctx)
		if err != nil {
			app.logger.Error(err.Error())
		}
	}
	return nil
}

func (app *application) runDeliveryWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)

	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return
		}
		if err := app.processPendingDeliveries(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			app.logger.Error("delivery worker failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
