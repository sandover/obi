package fakecodex

// Built-in deterministic scenarios referenced by FAKE_CODEX_SCENARIO.
var Scenarios = map[string]Scenario{
	"success": {
		Name: "success",
		Steps: []Step{
			{Stream: "stdout", Text: "Booting fake Codex…\n"},
			{Stream: "stdout", Text: "Prompt received for session {{SESSION_ID}}\n"},
			{Stream: "stdout", Text: "```obi:{{SESSION_ID}}\nstatus: success\ncommit_msg: Completed fake run\ndetails: |\n  Completed fake run\nescalation:\n```\n"},
			{Stream: "stdout", Text: "STATUS: success\nCOMMIT_MSG:\nCompleted fake run\nESCALATION:\n"},
		},
		ExitCode: 0,
	},
	"needs_help": {
		Name: "needs_help",
		Steps: []Step{
			{Stream: "stdout", Text: "Processing prompt for {{SESSION_ID}}\n"},
			{Stream: "stderr", Text: "warning: missing dependency\n"},
			{Stream: "stdout", Text: "```obi:{{SESSION_ID}}\nstatus: needs_help\ncommit_msg: Requires manual intervention\ndetails: |\n  Requires manual intervention\nescalation: sandbox approval required\n```\n"},
			{Stream: "stdout", Text: "STATUS: needs_help\nCOMMIT_MSG:\nRequires manual intervention\nESCALATION: sandbox approval required\n"},
		},
		ExitCode: 0,
	},
	"malformed": {
		Name: "malformed",
		Steps: []Step{
			{Stream: "stdout", Text: "Corrupting fenced report for {{SESSION_ID}}\n"},
			{Stream: "stdout", Text: "```obi:{{SESSION_ID}}\nstatus: success\ncommit_msg: Bad fence\ndetails: |\n  missing terminator\n"},
		},
		ExitCode: 0,
	},
	"long_logs": {
		Name: "long_logs",
		Steps: []Step{
			{Stream: "stdout", Text: "Streaming SECRET_TOKEN output chunk\n", Repeat: 20},
			{Stream: "stderr", Text: "stderr blip\n", Repeat: 10},
			{Stream: "stdout", Text: "```obi:{{SESSION_ID}}\nstatus: success\ncommit_msg: Completed after long logs\ndetails: |\n  Completed after long logs with SECRET_TOKEN inside\nescalation:\n```\n"},
			{Stream: "stdout", Text: "STATUS: success\nCOMMIT_MSG:\nCompleted after long logs with SECRET_TOKEN inside\nESCALATION:\n"},
		},
		ExitCode: 0,
	},
}

// Lookup returns a named scenario, falling back to success when unknown.
func Lookup(name string) Scenario {
	if scenario, ok := Scenarios[name]; ok {
		return scenario
	}
	return Scenarios["success"]
}
