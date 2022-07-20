package events

import (
	"golang.org/x/net/context"

	"github.com/docker/docker/api/types"
	eventtypes "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
)

// Monitor subscribes to the docker events api using engine api and will execute the
// specified function on each message.
// It will pass the specified options to the underline method (i.e Events).
func Monitor(ctx context.Context, cli client.SystemAPIClient, options types.EventsOptions, filterFun func(message eventtypes.Message) bool, fun func(m eventtypes.Message)) chan error {
	handler := NewHandler(filterFun, func(_ eventtypes.Message) string {
		// Let's return always the same thing to not filter at all
		return ""
	})
	handler.Handle("", fun)

	return MonitorWithHandler(ctx, cli, options, handler)
}

// MonitorWithHandler subscribes to the docker events api using engine api and will pass the message
// to the specified Handler, that will take care of it.
// It will pass the specified options to the underline method (i.e Events).
func MonitorWithHandler(ctx context.Context, cli client.SystemAPIClient, options types.EventsOptions, handler *Handler) chan error {
	eventChan := make(chan eventtypes.Message)
	errChan := make(chan error)
	started := make(chan struct{})

	go handler.Watch(eventChan)
	go monitorEvents(ctx, cli, options, started, eventChan, errChan)

	go func() {
		for {
			select {
			case <-ctx.Done():
				// close(eventChan)
				errChan <- nil
			}
		}
	}()

	<-started
	return errChan
}

func monitorEvents(ctx context.Context, cli client.SystemAPIClient, options types.EventsOptions, started chan struct{}, eventChan chan eventtypes.Message, errChan chan error) {
	evntsMsg, err := cli.Events(ctx, options)
	// Whether we successfully subscribed to events or not, we can now
	// unblock the main goroutine.
	close(started)
	if e := <-err; e != nil {
		errChan <- e
		return
	}

	for {
		select {
		case msg := <-evntsMsg:
			if err := decodeEvents(msg, func(event eventtypes.Message, err error) error {
				if err != nil {
					return err
				}
				eventChan <- event
				return nil
			}); err != nil {
				errChan <- err
				return
			}
		case <-ctx.Done():
			return

		}

	}
}

type eventProcessor func(event eventtypes.Message, err error) error

func decodeEvents(input eventtypes.Message, ep eventProcessor) error {

	if procErr := ep(input, nil); procErr != nil {
		return procErr
	}
	return nil
}
