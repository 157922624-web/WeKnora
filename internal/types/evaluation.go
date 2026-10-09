package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/yanyiwu/gojieba"
)

// Jieba is a global instance of Chinese text segmentation tool.
// May be nil when dictionaries are missing; callers must nil-check
// before calling Cut/CutForSearch (SegmentWithJieba guards this).
var Jieba *gojieba.Jieba = newJieba()

func jiebaDictsPresent(dir string) bool {
	if dir == "" {
		return false
	}
	for _, name := range []string{
		"jieba.dict.utf8",
		"hmm_model.utf8",
		"user.dict.utf8",
		"idf.utf8",
		"stop_words.utf8",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

func newJieba() *gojieba.Jieba {
	candidates := []string{}
	if dir := os.Getenv("JIEBA_DICT_DIR"); dir != "" {
		candidates = append(candidates, dir)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "jieba-dict"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "jieba-dict"))
	}

	for _, dir := range candidates {
		if !jiebaDictsPresent(dir) {
			continue
		}
		return gojieba.NewJieba(
			filepath.Join(dir, "jieba.dict.utf8"),
			filepath.Join(dir, "hmm_model.utf8"),
			filepath.Join(dir, "user.dict.utf8"),
			filepath.Join(dir, "idf.utf8"),
			filepath.Join(dir, "stop_words.utf8"),
		)
	}

	// Do not panic at init when dictionaries are absent (e.g. WebView-only
	// launch without CWD/jieba-dict). Chinese segmentation degrades to
	// no-op; English paths are unaffected.
	os.Stderr.WriteString("WARNING: gojieba dictionaries not found (JIEBA_DICT_DIR / exe-dir/jieba-dict / cwd/jieba-dict); Jieba is nil, Chinese segmentation disabled\n")
	return nil
}

// EvaluationStatue represents the status of an evaluation task
type EvaluationStatue int

const (
	EvaluationStatuePending EvaluationStatue = iota // Task is waiting to start
	EvaluationStatueRunning                         // Task is in progress
	EvaluationStatueSuccess                         // Task completed successfully
	EvaluationStatueFailed                          // Task failed
)

// EvaluationTask contains information about an evaluation task
type EvaluationTask struct {
	ID        string `json:"id"`         // Unique task ID
	TenantID  uint64 `json:"tenant_id"`  // Tenant/Organization ID
	DatasetID string `json:"dataset_id"` // Dataset ID for evaluation

	StartTime time.Time        `json:"start_time"`        // Task start time
	Status    EvaluationStatue `json:"status"`            // Current task status
	ErrMsg    string           `json:"err_msg,omitempty"` // Error message if failed

	Total    int `json:"total,omitempty"`    // Total items to evaluate
	Finished int `json:"finished,omitempty"` // Completed items count
}

// EvaluationDetail contains detailed evaluation information
type EvaluationDetail struct {
	Task   *EvaluationTask `json:"task"`             // Evaluation task info
	Params *ChatManage     `json:"params"`           // Evaluation parameters
	Metric *MetricResult   `json:"metric,omitempty"` // Evaluation metrics
}

// String returns JSON representation of EvaluationTask
func (e *EvaluationTask) String() string {
	b, _ := json.Marshal(e)
	return string(b)
}

// MetricInput contains input data for metric calculation
type MetricInput struct {
	RetrievalGT  [][]int // Ground truth for retrieval
	RetrievalIDs []int   // Retrieved IDs

	GeneratedTexts string // Generated text for evaluation
	GeneratedGT    string // Ground truth text for comparison
}

// MetricResult contains evaluation metrics
type MetricResult struct {
	RetrievalMetrics  RetrievalMetrics  `json:"retrieval_metrics"`  // Retrieval performance metrics
	GenerationMetrics GenerationMetrics `json:"generation_metrics"` // Text generation quality metrics
}

// RetrievalMetrics contains metrics for retrieval evaluation
type RetrievalMetrics struct {
	Precision float64 `json:"precision"` // Precision score
	Recall    float64 `json:"recall"`    // Recall score

	NDCG3  float64 `json:"ndcg3"`  // Normalized Discounted Cumulative Gain at 3
	NDCG10 float64 `json:"ndcg10"` // Normalized Discounted Cumulative Gain at 10
	MRR    float64 `json:"mrr"`    // Mean Reciprocal Rank
	MAP    float64 `json:"map"`    // Mean Average Precision
}

// GenerationMetrics contains metrics for text generation evaluation
type GenerationMetrics struct {
	BLEU1 float64 `json:"bleu1"` // BLEU-1 score
	BLEU2 float64 `json:"bleu2"` // BLEU-2 score
	BLEU4 float64 `json:"bleu4"` // BLEU-4 score

	ROUGE1 float64 `json:"rouge1"` // ROUGE-1 score
	ROUGE2 float64 `json:"rouge2"` // ROUGE-2 score
	ROUGEL float64 `json:"rougel"` // ROUGE-L score
}

// EvalState represents different stages of evaluation process
type EvalState int

const (
	StateBegin             EvalState = iota // Evaluation started
	StateAfterQaPairs                       // After loading QA pairs
	StateAfterDataset                       // After processing dataset
	StateAfterEmbedding                     // After generating embeddings
	StateAfterVectorSearch                  // After vector search
	StateAfterRerank                        // After reranking
	StateAfterComplete                      // After completion
	StateEnd                                // Evaluation ended
)
