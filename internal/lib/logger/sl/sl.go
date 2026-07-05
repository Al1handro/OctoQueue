package sl

import "log/slog"

func Err(err error) slog.Attr {
	if err == nil {
		return slog.Attr{
			Key:   "error",
			Value: slog.StringValue(""),
		}
	}

	return slog.Attr{
		Key:   "error",
		Value: slog.StringValue(err.Error()),
	}
}