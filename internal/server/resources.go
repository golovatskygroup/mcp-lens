package server

import (
	"encoding/json"
	"fmt"

	"github.com/golovatskygroup/mcp-lens/internal/artifacts"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type listResourcesResult struct {
	Resources []resource `json:"resources"`
}

type resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type readResourceParams struct {
	URI string `json:"uri"`
}

type readResourceResult struct {
	Contents []mcp.ContentBlock `json:"contents"`
}

func (s *Server) handleListResources(req *mcp.Request) *mcp.Response {
	st := s.handler.ArtifactStore()
	if st == nil {
		resp, _ := mcp.NewResponse(req.ID, listResourcesResult{Resources: nil})
		return resp
	}

	items := st.List()
	res := make([]resource, 0, len(items))
	for _, it := range items {
		res = append(res, resource{
			URI:      artifacts.ArtifactURI(it.ID),
			Name:     it.Tool,
			MimeType: it.Mime,
		})
	}

	resp, err := mcp.NewResponse(req.ID, listResourcesResult{Resources: res})
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.InternalError, err.Error())
	}
	return resp
}

func (s *Server) handleReadResource(req *mcp.Request) *mcp.Response {
	var params readResourceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.InvalidParams, "Invalid params: "+err.Error())
	}
	id, ok := artifacts.ParseArtifactURI(params.URI)
	if !ok {
		return mcp.NewErrorResponse(req.ID, mcp.InvalidParams, fmt.Sprintf("Unsupported resource URI: %s", params.URI))
	}

	st := s.handler.ArtifactStore()
	if st == nil {
		return mcp.NewErrorResponse(req.ID, mcp.InvalidParams, "No artifact store configured")
	}

	b, _, ok := st.Read(id)
	if !ok {
		return mcp.NewErrorResponse(req.ID, mcp.InvalidParams, "Resource not found")
	}

	result := readResourceResult{Contents: []mcp.ContentBlock{{Type: "text", Text: string(b), URI: params.URI}}}
	resp, err := mcp.NewResponse(req.ID, result)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.InternalError, err.Error())
	}
	return resp
}
