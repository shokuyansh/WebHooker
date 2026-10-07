package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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

var client = &http.Client{
	Transport: &http.Transport{
		DialContext: urlguard.Dialer(2 * time.Second).DialContext,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func signMessage(secret []byte, message []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(message)

	signature := h.Sum(nil)

	return base64.StdEncoding.EncodeToString(signature)
}

func (app *application) performDelivery(delivery *data.Delivery, ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	event, err := app.models.Events.Get(delivery.EventID)
	if err != nil {
		return fmt.Errorf("load event: %w", err)
	}

	webhook, err := app.models.Webhooks.GetWithSecret(int(event.ProjectID), int(delivery.WebHookID))
	if err != nil {
		return fmt.Errorf("load webhook: %w", err)
	}

	now := time.Now().UTC()
	delivery.LastAttemptAt = &now

	timeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(timeCtx, http.MethodPost, webhook.CallbackURL, bytes.NewBuffer(event.Payload))
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	delivery_id := strconv.FormatInt(delivery.ID, 10)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	message := delivery_id + "." + timestamp + "." + string(event.Payload)
	signature := signMessage(webhook.SigningSecret, []byte(message))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("webhook-id", delivery_id)
	req.Header.Set("webhook-timestamp", timestamp)
	req.Header.Set("webhook-signature", "v1,"+signature)

	resp, err := client.Do(req)
	if err != nil {
		switch {
		case errors.Is(err, urlguard.ErrBlockedDestination):
			app.logger.Error("delivery processing failed",
				"delivery_id", delivery.ID,
				"webhook_id", delivery.WebHookID,
				"event_id", delivery.EventID,
				"error", err,
			)
		case ctx.Err() != nil:
			return ctx.Err()
		default:
			app.logger.Error("delivery request failed",
				"delivery_id", delivery.ID,
				"webhook_id", delivery.WebHookID,
				"event_id", delivery.EventID,
				"error", err,
			)
		}
		delivery.ResponseStatus = nil
		delivery.Status = "FAILED"
	} else {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			delivery.Status = "SUCCESS"
			delivery.ResponseStatus = &resp.StatusCode
		} else {
			app.logger.Warn("delivery response non-2xx:", "delivery_id", delivery.ID,
				"webhook_id", delivery.WebHookID,
				"event_id", delivery.EventID,
				"response_status", resp.StatusCode)
			delivery.Status = "FAILED"
			delivery.ResponseStatus = &resp.StatusCode
		}
		resp.Body.Close()
	}
	err = app.models.Deliveries.Update(delivery)
	if err != nil {
		return fmt.Errorf("update delivery: %w", err)
	}
	return nil
}

func (app *application) processPendingDeliveries(ctx context.Context) error {
	pendingDeliveries, err := app.models.Deliveries.PendingDeliveries()
	if err != nil {
		return err
	}
	for _, delivery := range pendingDeliveries {
		err := app.performDelivery(delivery, ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			app.logger.Error("delivery processing failed",
				"delivery_id", delivery.ID,
				"webhook_id", delivery.WebHookID,
				"event_id", delivery.EventID,
				"error", err,
			)
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
