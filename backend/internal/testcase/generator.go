package testcase

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/requirement"
)

var documentedNumberPattern = regexp.MustCompile(`\b\d+(?:[.,]\d+)?(?:\s*(?:%|ms|s|phút|giây|ngày|VNĐ|VND))?\b`)

type Generator struct{}

func NewGenerator() *Generator { return &Generator{} }

func (g *Generator) Generate(source requirement.Detail) []Proposal {
	req := source.Requirement
	if req.Status != requirement.StatusApproved || len(source.Evidence) == 0 {
		return nil
	}
	baseType := TypeHappy
	if req.FlowType == "EXCEPTION" || containsAny(req.Statement,
		"không hợp lệ", "không được", "từ chối", "lỗi", "failed", "invalid", "reject") {
		baseType = TypeNegative
	}
	proposals := []Proposal{g.proposal(source, baseType, baseTitle(baseType, req.Title))}
	if documentedNumberPattern.MatchString(req.Statement) {
		proposals = append(proposals, g.proposal(source, TypeBoundary,
			"Kiểm tra ngưỡng được mô tả: "+req.Title))
	}
	if req.Actor != "" && containsAny(req.Statement+" "+req.Title,
		"chỉ được", "chỉ có", "quyền", "vai trò", "role", "permission", "authorized", "được phép") {
		proposals = append(proposals, g.proposal(source, TypePermission,
			"Kiểm tra quyền của "+req.Actor+": "+req.Title))
	}
	if req.Postcondition != "" || containsAny(req.Statement+" "+req.Title,
		"trạng thái", "chuyển trạng thái", "state", "status") {
		proposals = append(proposals, g.proposal(source, TypeState,
			"Kiểm tra chuyển trạng thái: "+req.Title))
	}
	if containsAny(req.Statement+" "+req.Title+" "+req.Actor,
		"api", "database", "cơ sở dữ liệu", "dịch vụ", "service", "tích hợp", "integration", "provider") {
		proposals = append(proposals, g.proposal(source, TypeIntegration,
			"Kiểm tra tích hợp: "+req.Title))
	}
	if req.RequirementType == requirement.TypeRegression {
		proposals = append(proposals, g.proposal(source, TypeRegression,
			"Kiểm tra hồi quy: "+req.Title))
	}
	if req.RequirementType == requirement.TypeNonFunctional {
		proposals = append(proposals, g.proposal(source, TypeNFR,
			"Kiểm tra yêu cầu phi chức năng: "+req.Title))
	}
	return dedupeProposals(proposals)
}

func (g *Generator) proposal(source requirement.Detail, testType, title string) Proposal {
	req := source.Requirement
	steps := make([]Step, 0, len(source.FlowSteps)+2)
	if req.Precondition != "" {
		steps = append(steps, Step{Action: "Thiết lập tiền điều kiện: " + req.Precondition,
			ExpectedResult: "Tiền điều kiện được thiết lập thành công"})
	}
	for _, flowStep := range source.FlowSteps {
		steps = append(steps, Step{Action: flowStep.Action, ExpectedResult: flowStep.ExpectedResult})
	}
	if len(source.FlowSteps) == 0 {
		steps = append(steps, Step{Action: "Thực hiện hành vi được mô tả trong requirement " + req.RequirementKey,
			ExpectedResult: "Hành vi được thực hiện để có thể đối chiếu kết quả"})
	}
	steps = append(steps, Step{Action: "Đối chiếu kết quả với requirement đã duyệt",
		ExpectedResult: req.Statement})
	return Proposal{Title: title, TestType: testType, Risk: req.Risk, Actor: req.Actor,
		Precondition: req.Precondition, ExpectedResult: req.Statement,
		Postcondition: req.Postcondition, AutomationStatus: "MANUAL",
		Confidence: req.Confidence, GeneratedBy: "RULE_ENGINE", Steps: steps,
		RequirementIDs: []int64{req.ID}, RequirementKeys: []string{req.RequirementKey}}
}

func baseTitle(testType, title string) string {
	prefix := "Luồng hợp lệ"
	if testType == TypeNegative {
		prefix = "Luồng từ chối/ngoại lệ"
	}
	return prefix + ": " + title
}

func containsAny(value string, needles ...string) bool {
	value = strings.ToLower(value)
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func dedupeProposals(items []Proposal) []Proposal {
	seen := make(map[string]struct{}, len(items))
	result := make([]Proposal, 0, len(items))
	for _, item := range items {
		key := proposalIdentity(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func proposalIdentity(item Proposal) string {
	// Compare all business fields and exact source revision IDs. Case and test-data
	// whitespace may be meaningful; conservatively keep differences independent.
	// Step database IDs/timestamps are not business content; array order is.
	steps := make([]StepInput, 0, len(item.Steps))
	for _, step := range item.Steps {
		steps = append(steps, StepInput{Action: step.Action, ExpectedResult: step.ExpectedResult})
	}
	assumptions := append([]string{}, item.Assumptions...)
	sort.Strings(assumptions)
	content := RevisionContent{Title: item.Title, TestType: item.TestType, Risk: item.Risk,
		Actor: item.Actor, Precondition: item.Precondition, TestData: item.TestData,
		ExpectedResult: item.ExpectedResult, Postcondition: item.Postcondition,
		Steps: steps, Assumptions: assumptions, RequirementRevisionIDs: uniqueSortedIDs(item.RequirementIDs)}
	var provenance GenerationProvenance
	if item.Generation != nil {
		provenance = *item.Generation
		// Attempt/job bookkeeping is not a new scenario or source revision.
		provenance.WorkflowJobID, provenance.WorkflowInputHash = 0, ""
	}
	payload, _ := json.Marshal(struct {
		Content                       RevisionContent
		GeneratedBy, AutomationStatus string
		Confidence                    float64
		Generation                    GenerationProvenance
	}{content, item.GeneratedBy, item.AutomationStatus, item.Confidence, provenance})
	return hash(string(payload))
}

func normalize(value string) string { return strings.ToLower(strings.Join(strings.Fields(value), " ")) }

func uniqueInt64(values []int64) []int64 {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
