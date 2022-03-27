// Copyright 2022 The jackal Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adminserver

import (
	"context"
	"net"
	"strconv"
	"sync/atomic"

	kitlog "github.com/go-kit/log"

	"github.com/go-kit/log/level"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	adminpb "github.com/ortuman/jackal/pkg/admin/pb"
	"github.com/ortuman/jackal/pkg/auth/pepper"
	"github.com/ortuman/jackal/pkg/hook"
	"github.com/ortuman/jackal/pkg/storage/repository"
	"google.golang.org/grpc"
)

var netListen = net.Listen

// Server represents an admin server type.
type Server struct {
	bindAddr string
	port     int
	ln       net.Listener
	active   int32

	rep     repository.Repository
	peppers *pepper.Keys
	hk      *hook.Hooks
	logger  kitlog.Logger
}

// Config contains Server configuration parameters.
type Config struct {
	BindAddr string `fig:"bind_addr"`
	Port     int    `fig:"port" default:"15280"`
	Disabled bool   `fig:"disabled"`
}

// New returns a new initialized admin server.
func New(
	cfg Config,
	rep repository.Repository,
	peppers *pepper.Keys,
	hk *hook.Hooks,
	logger kitlog.Logger,
) *Server {
	if cfg.Disabled {
		return nil
	}
	return &Server{
		bindAddr: cfg.BindAddr,
		port:     cfg.Port,
		rep:      rep,
		peppers:  peppers,
		hk:       hk,
		logger:   logger,
	}
}

// Start starts admin server.
func (s *Server) Start(_ context.Context) error {
	addr := s.getAddress()

	ln, err := netListen("tcp", addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.active = 1

	level.Info(s.logger).Log("msg", "started admin server", "bind_addr", addr)

	go func() {
		grpcServer := grpc.NewServer(
			grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
			grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
		)
		adminpb.RegisterUsersServer(grpcServer, newUsersService(s.rep, s.peppers, s.hk, s.logger))
		if err := grpcServer.Serve(s.ln); err != nil {
			if atomic.LoadInt32(&s.active) == 1 {
				level.Error(s.logger).Log("msg", "admin server error", "err", err)
			}
		}
	}()
	return nil
}

// Stop stops admin server.
func (s *Server) Stop(_ context.Context) error {
	atomic.StoreInt32(&s.active, 0)
	if err := s.ln.Close(); err != nil {
		return err
	}
	level.Info(s.logger).Log("msg", "closed admin server", "bind_addr", s.getAddress())
	return nil
}

func (s *Server) getAddress() string {
	return s.bindAddr + ":" + strconv.Itoa(s.port)
}
