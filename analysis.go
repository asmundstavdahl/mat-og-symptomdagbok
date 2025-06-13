package main

import (
	"math"
	"sort"
)

// lowPassFilter applies a first-order low-pass filter to a time series.
// y[n] = alpha * x[n] + (1-alpha) * y[n-1]
// alpha = dt / (tau + dt)
// tau: time constant in minutes
// dt: time resolution in minutes (here always 1)
func lowPassFilter(series []int, tau float64) []float64 {
	if tau <= 0 {
		out := make([]float64, len(series))
		for i, v := range series {
			out[i] = float64(v)
		}
		return out
	}
	alpha := 1.0 / (tau + 1.0)
	out := make([]float64, len(series))
	if len(series) == 0 {
		return out
	}
	out[0] = float64(series[0])
	for i := 1; i < len(series); i++ {
		out[i] = alpha*float64(series[i]) + (1.0-alpha)*out[i-1]
	}
	return out
}

// crossCorrelation computes the cross-correlation between two binary time series (meal/symptom).
// Returns a slice of correlation values for lags from -maxLag to +maxLag.
func crossCorrelation(x, y []float64, maxLag int) ([]int, []float64) {
	n := len(x)
	cc := make([]float64, 2*maxLag+1)
	lags := make([]int, 2*maxLag+1)
	for lag := -maxLag; lag <= maxLag; lag++ {
		var sum float64
		var count int
		for i := 0; i < n; i++ {
			j := i + lag
			if j < 0 || j >= n {
				continue
			}
			sum += x[i] * y[j]
			count++
		}
		if count > 0 {
			cc[lag+maxLag] = sum / float64(count)
		}
		lags[lag+maxLag] = lag
	}
	return lags, cc
}
