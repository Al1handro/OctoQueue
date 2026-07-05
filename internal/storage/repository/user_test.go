package repository

import (
	"OctoQueue/internal/domain"
	"context"
	"testing"
)

func Test_pgUserRepo_CreateUser(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		user    *domain.User
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: construct the receiver type.
			var s pgUserRepo
			gotErr := s.CreateUser(context.Background(), tt.user)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("CreateUser() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("CreateUser() succeeded unexpectedly")
			}
		})
	}
}

func Test_pgUserRepo_GetUser(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		id      string
		want    *domain.User
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: construct the receiver type.
			var s pgUserRepo
			got, gotErr := s.GetUser(context.Background(), tt.id)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("GetUser() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("GetUser() succeeded unexpectedly")
			}
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("GetUser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_pgUserRepo_GetByEmail(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		email   string
		want    *domain.User
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: construct the receiver type.
			var s pgUserRepo
			got, gotErr := s.GetByEmail(context.Background(), tt.email)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("GetByEmail() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("GetByEmail() succeeded unexpectedly")
			}
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("GetByEmail() = %v, want %v", got, tt.want)
			}
		})
	}
}
