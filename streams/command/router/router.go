package router

import (
	"context"
	"errors"

	"github.com/taubyte/p2p/streams"
	"github.com/taubyte/p2p/streams/command"
	ce "github.com/taubyte/p2p/streams/command/error"
	cr "github.com/taubyte/p2p/streams/command/response"
)

type CommandHandler func(context.Context, streams.Connection, command.Body) (cr.Response, error)

type RouteHandler func(command string) CommandHandler

type Router struct {
	svr          *streams.StreamManger
	staticRoutes map[string]CommandHandler
	routeHandler RouteHandler
}

func New(svr *streams.StreamManger, routeHandler RouteHandler) *Router {
	if routeHandler == nil {
		routeHandler = NilRouteHandler
	}

	return &Router{svr: svr, staticRoutes: map[string]CommandHandler{}, routeHandler: routeHandler}
}

func (r *Router) AddStatic(command string, handler CommandHandler) error {
	if handler == nil {
		return errors.New("can not add nil handler")
	}

	if _, ok := r.staticRoutes[command]; ok {
		return errors.New("Command `" + command + "` already exists.")
	}

	r.staticRoutes[command] = handler
	return nil
}

func (r *Router) Handle(cmd *command.Command) (cr.Response, error) {
	if cmd == nil {
		return nil, errors.New("empty command")
	}

	conn, err := cmd.Connection()
	if err != nil {
		return nil, err
	}

	if handler, ok := r.staticRoutes[cmd.Command]; ok {
		return handler(r.svr.Context(), conn, cmd.Body)
	}

	if handler := r.routeHandler(cmd.Command); handler != nil {
		return handler(r.svr.Context(), conn, cmd.Body)
	}

	return nil, errors.New("command `" + cmd.Command + "` does not exist.")
}

func (r *Router) HandleRaw(s streams.Stream) {
	defer s.Close()
	c, err := command.Decode(s.Conn(), s)
	if err != nil {
		ce.Encode(s, err)
		return
	}

	creturn, err := r.Handle(c)
	if err != nil {
		ce.Encode(s, err)
		return
	}

	if err = creturn.Encode(s); err != nil {
		ce.Encode(s, err)
	}

}
