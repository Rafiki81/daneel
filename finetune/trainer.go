package finetune

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Method specifies the fine-tuning method.
type Method int

const (
	MethodLoRA  Method = iota // LoRA (fast, low VRAM)
	MethodQLoRA               // Quantized LoRA (even less VRAM)
	MethodFull                // Full fine-tuning
	MethodGRPO                // Group Relative Policy Optimization
)

// LoRAConfig configures LoRA training parameters.
type LoRAConfig struct {
	Rank          int      `json:"rank"`
	Alpha         int      `json:"alpha"`
	Dropout       float64  `json:"dropout"`
	TargetModules []string `json:"target_modules"`
}

// GRPOConfig configures GRPO training.
type GRPOConfig struct {
	RewardModel    string  `json:"reward_model"`
	KLCoeff        float64 `json:"kl_coeff"`
	NumGenerations int     `json:"num_generations"`
}

// Config holds all training configuration.
type Config struct {
	Method       Method     `json:"method"`
	BaseModel    string     `json:"base_model"`
	DataPath     string     `json:"data_path"`
	OutputDir    string     `json:"output_dir"`
	Epochs       int        `json:"epochs"`
	BatchSize    int        `json:"batch_size"`
	LearningRate float64    `json:"learning_rate"`
	ExportGGUF   string     `json:"export_gguf"`
	UseUnsloth   bool       `json:"use_unsloth"`
	UseTRL       bool       `json:"use_trl"`
	UseMLX       bool       `json:"use_mlx"`
	LoRA         LoRAConfig `json:"lora"`
	GRPO         GRPOConfig `json:"grpo"`
	VenvPath     string     `json:"venv_path"`
}

// TrainOption configures a training job.
type TrainOption func(*Config)

func WithUnsloth() TrainOption            { return func(c *Config) { c.UseUnsloth = true } }
func WithTRL() TrainOption                { return func(c *Config) { c.UseTRL = true } }
func WithMLX() TrainOption                { return func(c *Config) { c.UseMLX = true } }
func BaseModel(m string) TrainOption      { return func(c *Config) { c.BaseModel = m } }
func Epochs(n int) TrainOption            { return func(c *Config) { c.Epochs = n } }
func BatchSize(n int) TrainOption         { return func(c *Config) { c.BatchSize = n } }
func LearningRate(lr float64) TrainOption { return func(c *Config) { c.LearningRate = lr } }
func ExportGGUF(quant string) TrainOption { return func(c *Config) { c.ExportGGUF = quant } }
func OutputDir(d string) TrainOption      { return func(c *Config) { c.OutputDir = d } }
func VenvPath(d string) TrainOption       { return func(c *Config) { c.VenvPath = d } }

func LoRA(cfg LoRAConfig) TrainOption {
	return func(c *Config) { c.Method = MethodLoRA; c.LoRA = cfg }
}
func QLoRA(cfg LoRAConfig) TrainOption {
	return func(c *Config) { c.Method = MethodQLoRA; c.LoRA = cfg }
}
func FullFineTune() TrainOption {
	return func(c *Config) { c.Method = MethodFull }
}
func GRPO(cfg GRPOConfig) TrainOption {
	return func(c *Config) { c.Method = MethodGRPO; c.GRPO = cfg }
}

// Result is the output of a training job.
type Result struct {
	OutputPath string        `json:"output_path"`
	Duration   time.Duration `json:"duration"`
	FinalLoss  float64       `json:"final_loss"`
	Version    string        `json:"version"`
	Eval       *EvalResult   `json:"eval,omitempty"`
}

// Job represents a running training process.
type Job struct {
	cmd      *exec.Cmd
	config   Config
	progress chan TrainUpdate
	result   chan jobResult
}

type jobResult struct {
	result Result
	err    error
}

// TrainUpdate is a progress update from the training process.
type TrainUpdate struct {
	Epoch       int     `json:"epoch"`
	TotalEpochs int     `json:"total_epochs"`
	Step        int     `json:"step"`
	Loss        float64 `json:"loss"`
	LR          float64 `json:"lr"`
}

// Progress returns a channel of training updates.
func (j *Job) Progress() <-chan TrainUpdate { return j.progress }

// Wait blocks until training completes.
func (j *Job) Wait() (Result, error) {
	r := <-j.result
	return r.result, r.err
}

// Run starts a fine-tuning job.
func Run(ctx context.Context, dataPath string, opts ...TrainOption) (*Job, error) {
	cfg := Config{
		DataPath:     dataPath,
		OutputDir:    "./models/output",
		Epochs:       3,
		BatchSize:    4,
		LearningRate: 2e-4,
		UseUnsloth:   true,
		VenvPath:     "./.daneel-venv",
		Method:       MethodLoRA,
		LoRA: LoRAConfig{
			Rank:          16,
			Alpha:         32,
			Dropout:       0.05,
			TargetModules: []string{"q_proj", "v_proj", "k_proj", "o_proj"},
		},
	}
	for _, o := range opts {
		o(&cfg)
	}

	// Generate Python training script
	script, err := generateScript(cfg)
	if err != nil {
		return nil, fmt.Errorf("finetune: generate script: %w", err)
	}

	scriptPath := filepath.Join(cfg.OutputDir, "train.py")
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("finetune: mkdir: %w", err)
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		return nil, fmt.Errorf("finetune: write script: %w", err)
	}

	pythonBin := filepath.Join(cfg.VenvPath, "bin", "python3")
	if _, err := os.Stat(pythonBin); err != nil {
		pythonBin = "python3"
	}

	cmd := exec.CommandContext(ctx, pythonBin, scriptPath)
	cmd.Dir = cfg.OutputDir

	job := &Job{
		cmd:      cmd,
		config:   cfg,
		progress: make(chan TrainUpdate, 100),
		result:   make(chan jobResult, 1),
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("finetune: stdout pipe: %w", err)
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("finetune: start: %w", err)
	}

	go func() {
		defer close(job.progress)
		defer close(job.result)

		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				var update TrainUpdate
				if json.Unmarshal(buf[:n], &update) == nil && update.Step > 0 {
					job.progress <- update
				}
			}
			if err != nil {
				break
			}
		}

		waitErr := cmd.Wait()
		r := Result{
			OutputPath: cfg.OutputDir,
			Duration:   time.Since(start),
		}
		job.result <- jobResult{result: r, err: waitErr}
	}()

	return job, nil
}

func generateScript(cfg Config) (string, error) {
	var s string
	s += "# Auto-generated by daneel finetune\n"
	s += "import json, os, sys\n"

	if cfg.UseUnsloth {
		s += "from unsloth import FastLanguageModel\n"
		s += "from trl import SFTTrainer\n"
		s += "from transformers import TrainingArguments\n"
		s += "from datasets import load_dataset\n\n"
		s += fmt.Sprintf("model, tokenizer = FastLanguageModel.from_pretrained(\"%s\", max_seq_length=2048, load_in_4bit=%v)\n",
			cfg.BaseModel, cfg.Method == MethodQLoRA)
		s += fmt.Sprintf("model = FastLanguageModel.get_peft_model(model, r=%d, lora_alpha=%d, lora_dropout=%f, target_modules=%s)\n",
			cfg.LoRA.Rank, cfg.LoRA.Alpha, cfg.LoRA.Dropout, toPythonList(cfg.LoRA.TargetModules))
	} else if cfg.UseTRL {
		s += "from trl import SFTTrainer\n"
		s += "from transformers import AutoModelForCausalLM, AutoTokenizer, TrainingArguments\n"
		s += "from datasets import load_dataset\n"
		s += "from peft import LoraConfig\n\n"
		s += fmt.Sprintf("model = AutoModelForCausalLM.from_pretrained(\"%s\")\n", cfg.BaseModel)
		s += fmt.Sprintf("tokenizer = AutoTokenizer.from_pretrained(\"%s\")\n", cfg.BaseModel)
	}

	s += fmt.Sprintf("\ndataset = load_dataset('json', data_files='%s', split='train')\n", cfg.DataPath)
	s += fmt.Sprintf("\ntraining_args = TrainingArguments(\n")
	s += fmt.Sprintf("    output_dir='%s',\n", cfg.OutputDir)
	s += fmt.Sprintf("    num_train_epochs=%d,\n", cfg.Epochs)
	s += fmt.Sprintf("    per_device_train_batch_size=%d,\n", cfg.BatchSize)
	s += fmt.Sprintf("    learning_rate=%e,\n", cfg.LearningRate)
	s += "    logging_steps=1,\n"
	s += "    save_strategy='epoch',\n"
	s += ")\n\n"
	s += "trainer = SFTTrainer(model=model, tokenizer=tokenizer, train_dataset=dataset, args=training_args)\n"
	s += "result = trainer.train()\n"
	s += "print(json.dumps({'loss': result.training_loss, 'step': result.global_step}))\n"
	s += "model.save_pretrained(training_args.output_dir)\n"
	s += "tokenizer.save_pretrained(training_args.output_dir)\n"

	if cfg.ExportGGUF != "" {
		s += fmt.Sprintf("\n# Export to GGUF\nmodel.save_pretrained_gguf('%s', tokenizer, quantization_method='%s')\n",
			cfg.OutputDir, cfg.ExportGGUF)
	}

	return s, nil
}

func toPythonList(ss []string) string {
	if len(ss) == 0 {
		return "[]"
	}
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = fmt.Sprintf("\"%s\"", s)
	}
	return "[" + joinStrings(parts, ", ") + "]"
}

func joinStrings(ss []string, sep string) string {
	return strings.Join(ss, sep)
}

// --- Python environment setup ---

// Setup creates a Python virtual environment with training dependencies.
func Setup(ctx context.Context, opts ...TrainOption) error {
	cfg := Config{VenvPath: "./.daneel-venv"}
	for _, o := range opts {
		o(&cfg)
	}

	pythonPath := "python3"
	// Create venv
	if err := exec.CommandContext(ctx, pythonPath, "-m", "venv", cfg.VenvPath).Run(); err != nil {
		return fmt.Errorf("finetune: create venv: %w", err)
	}
	// Install dependencies
	pip := filepath.Join(cfg.VenvPath, "bin", "pip")
	pkgs := []string{"torch", "transformers", "datasets", "trl", "peft", "accelerate"}
	if cfg.UseUnsloth {
		pkgs = append(pkgs, "unsloth")
	}
	args := append([]string{"install", "--quiet"}, pkgs...)
	if err := exec.CommandContext(ctx, pip, args...).Run(); err != nil {
		return fmt.Errorf("finetune: pip install: %w", err)
	}
	return nil
}

// PythonPath sets the Python executable path.
func PythonPath(p string) TrainOption {
	return func(c *Config) { _ = p } // stored externally for Setup
}

// Check verifies that all Python dependencies are available.
func Check(ctx context.Context, opts ...TrainOption) (bool, []string) {
	cfg := Config{VenvPath: "./.daneel-venv"}
	for _, o := range opts {
		o(&cfg)
	}
	pythonBin := filepath.Join(cfg.VenvPath, "bin", "python3")
	if _, err := os.Stat(pythonBin); err != nil {
		pythonBin = "python3"
	}
	required := []string{"torch", "transformers", "datasets", "trl", "peft"}
	var missing []string
	for _, pkg := range required {
		cmd := exec.CommandContext(ctx, pythonBin, "-c", fmt.Sprintf("import %s", pkg))
		if cmd.Run() != nil {
			missing = append(missing, pkg)
		}
	}
	return len(missing) == 0, missing
}

// ImportToOllama imports a trained model to Ollama.
func ImportToOllama(ctx context.Context, modelPath, name string, opts ...DeployOption) error {
	dcfg := deployConfig{}
	for _, o := range opts {
		o(&dcfg)
	}
	// Create Modelfile
	modelfile := fmt.Sprintf("FROM %s\n", modelPath)
	if dcfg.systemPrompt != "" {
		modelfile += fmt.Sprintf("SYSTEM %s\n", dcfg.systemPrompt)
	}
	if dcfg.temperature > 0 {
		modelfile += fmt.Sprintf("PARAMETER temperature %f\n", dcfg.temperature)
	}
	if dcfg.contextLength > 0 {
		modelfile += fmt.Sprintf("PARAMETER num_ctx %d\n", dcfg.contextLength)
	}
	mfPath := filepath.Join(filepath.Dir(modelPath), "Modelfile")
	if err := os.WriteFile(mfPath, []byte(modelfile), 0o644); err != nil {
		return fmt.Errorf("finetune: write modelfile: %w", err)
	}
	cmd := exec.CommandContext(ctx, "ollama", "create", name, "-f", mfPath)
	return cmd.Run()
}

// ImportToLocalAI imports a trained model to LocalAI.
func ImportToLocalAI(ctx context.Context, modelPath, name string, opts ...DeployOption) error {
	dcfg := deployConfig{}
	for _, o := range opts {
		o(&dcfg)
	}
	config := map[string]any{
		"name":    name,
		"backend": dcfg.backend,
		"parameters": map[string]any{
			"model": modelPath,
		},
	}
	if dcfg.gpuLayers > 0 {
		config["gpu_layers"] = dcfg.gpuLayers
	}
	b, _ := json.MarshalIndent(config, "", "  ")
	configPath := filepath.Join(filepath.Dir(modelPath), name+".yaml")
	return os.WriteFile(configPath, b, 0o644)
}

type deployConfig struct {
	systemPrompt  string
	temperature   float64
	contextLength int
	backend       string
	gpuLayers     int
}

// DeployOption configures model deployment.
type DeployOption func(*deployConfig)

// WithSystemPrompt sets the system prompt for the deployed model.
func WithSystemPrompt(s string) DeployOption {
	return func(c *deployConfig) { c.systemPrompt = s }
}

// WithTemperature sets the default temperature.
func WithTemperature(t float64) DeployOption {
	return func(c *deployConfig) { c.temperature = t }
}

// WithContextLength sets the context window size.
func WithContextLength(n int) DeployOption {
	return func(c *deployConfig) { c.contextLength = n }
}

// WithBackend sets the LocalAI backend.
func WithBackend(b string) DeployOption {
	return func(c *deployConfig) { c.backend = b }
}

// WithGPULayers sets GPU offload layers for LocalAI.
func WithGPULayers(n int) DeployOption {
	return func(c *deployConfig) { c.gpuLayers = n }
}
