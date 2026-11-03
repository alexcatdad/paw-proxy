//go:build darwin

package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestRestartArgs(t *testing.T) {
	tests := []struct {
		name   string
		loaded map[string]bool
		want   []string
		err    error
	}{
		{"app agent", map[string]bool{"gui/501/com.alexcatdad.paw-proxy.daemon": true}, []string{"kickstart", "-k", "gui/501/com.alexcatdad.paw-proxy.daemon"}, nil},
		{"legacy agent", map[string]bool{"gui/501/dev.paw-proxy": true}, []string{"kickstart", "-k", "gui/501/dev.paw-proxy"}, nil},
		{"both prefers app", map[string]bool{"gui/501/com.alexcatdad.paw-proxy.daemon": true, "gui/501/dev.paw-proxy": true}, []string{"kickstart", "-k", "gui/501/com.alexcatdad.paw-proxy.daemon"}, nil},
		{"none", map[string]bool{}, nil, errNoService},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := restartArgs(501, func(target string) bool { return tt.loaded[target] })
			if !errors.Is(err, tt.err) || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, %v; want %v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}
