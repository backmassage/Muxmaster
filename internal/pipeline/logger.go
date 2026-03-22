// Logger interface for dependency injection and testability.
package pipeline

// Logger is the logging interface used by the pipeline package. The concrete
// *logging.Logger satisfies this interface; tests can substitute a lightweight
// mock to verify orchestration, retry loops, and quality escalation without
// requiring real stdout/stderr or a log file.
type Logger interface {
	Info(string, ...interface{})
	Success(string, ...interface{})
	Warn(string, ...interface{})
	Error(string, ...interface{})
	Debug(bool, string, ...interface{})
	Outlier(string, ...interface{})
	Blank()
}
