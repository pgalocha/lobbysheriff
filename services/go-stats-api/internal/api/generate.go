// Package api contém o código gerado a partir do contrato OpenAPI
// (contracts/openapi/go-stats-api.yaml). Não editar api.gen.go à mão:
//
//	go generate ./...
//
// A CI falha se o código gerado não corresponder ao contrato.
package api

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../../contracts/openapi/go-stats-api.yaml
