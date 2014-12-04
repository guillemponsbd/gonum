// Copyright ©2014 The gonum Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package optimize

import (
	"github.com/gonum/floats"
	"github.com/gonum/matrix/mat64"
)

// BFGS implements the Method interface to perform the Broyden–Fletcher–Goldfarb–Shanno
// optimization method with the given linesearch method. If LinesearchMethod is nil,
// it will be set to a reasonable default.
//
// BFGS is a quasi-Newton method that performs successive rank-one updates to
// an estimate of the inverse-Hessian of the function. It exhibits super-linear
// convergence when in proximity to a local minimum. It has memory cost that is
// O(n^2) relative to the input dimension.
type BFGS struct {
	LinesearchMethod LinesearchMethod

	linesearch *Linesearch

	x    []float64 // location of the last major iteration
	grad []float64 // gradient at the last major iteration
	dim  int

	// Temporary memory
	direction []float64
	y         []float64
	s         []float64

	invHess *mat64.Dense // TODO: Make symmetric when mat64 has symmetric matrices

	first bool // Is it the first iteration (used to set the scale of the initial hessian)
}

// NOTE: This method exists so that it's easier to use a bfgs algorithm because
// it implements Method

func (b *BFGS) Init(l Location, f *FunctionInfo, xNext []float64) (EvaluationType, IterationType, error) {
	if b.LinesearchMethod == nil {
		b.LinesearchMethod = &Bisection{}
	}
	if b.linesearch == nil {
		b.linesearch = &Linesearch{}
	}
	b.linesearch.Method = b.LinesearchMethod
	b.linesearch.NextDirectioner = b

	return b.linesearch.Init(l, f, xNext)
}

func (b *BFGS) Iterate(l Location, xNext []float64) (EvaluationType, IterationType, error) {
	return b.linesearch.Iterate(l, xNext)
}

func (b *BFGS) InitDirection(l Location, dir []float64) (stepSize float64) {
	dim := len(l.X)
	b.dim = dim

	b.x = resize(b.x, dim)
	copy(b.x, l.X)
	b.grad = resize(b.grad, dim)
	copy(b.grad, l.Gradient)

	b.y = resize(b.y, dim)
	b.s = resize(b.s, dim)

	if b.invHess == nil || cap(b.invHess.RawMatrix().Data) < dim*dim {
		b.invHess = mat64.NewDense(dim, dim, nil)
	} else {
		b.invHess = mat64.NewDense(dim, dim, b.invHess.RawMatrix().Data[:dim*dim])
	}

	// The values of the hessian are initialized in the first call to NextDirection

	// initial direcion is just negative of gradient because the hessian is 1
	copy(dir, l.Gradient)
	floats.Scale(-1, dir)

	b.first = true

	return 1 / floats.Norm(dir, 2)
}

func (b *BFGS) NextDirection(l Location, direction []float64) (stepSize float64) {
	if len(l.X) != b.dim {
		panic("bfgs: unexpected size mismatch")
	}
	if len(l.Gradient) != b.dim {
		panic("bfgs: unexpected size mismatch")
	}
	if len(direction) != b.dim {
		panic("bfgs: unexpected size mismatch")
	}

	// Compute the gradient difference in the last step
	// y = g_{k+1} - g_{k}
	floats.SubTo(b.y, l.Gradient, b.grad)

	// Compute the step difference
	// s = x_{k+1} - x_{k}
	floats.SubTo(b.s, l.X, b.x)

	sDotY := floats.Dot(b.s, b.y)
	sDotYSquared := sDotY * sDotY

	if b.first {
		// Rescale the initial hessian.
		// From: Numerical optimization, Nocedal and Wright, Page 143, Eq. 6.20 (second edition).
		yDotY := floats.Dot(b.y, b.y)
		scale := sDotY / yDotY
		for i := 0; i < len(l.X); i++ {
			for j := 0; j < len(l.X); j++ {
				if i == j {
					b.invHess.Set(i, i, scale)
				} else {
					b.invHess.Set(i, j, 0)
				}
			}
		}
		b.first = false
	}

	// Compute the update rule
	//     B_{k+1}^-1
	// First term is just the existing inverse hessian
	// Second term is
	//     (sk^T yk + yk^T B_k^-1 yk)(s_k sk_^T) / (sk^T yk)^2
	// Third term is
	//     B_k ^-1 y_k sk^T + s_k y_k^T B_k-1

	// y_k^T B_k^-1 y_k is a scalar. Compute it.
	yBy := mat64.Inner(b.y, b.invHess, b.y)
	firstTermConst := (sDotY + yBy) / (sDotYSquared)

	// Compute the third term.
	// TODO: Replace this with Symmetric Rank 2 update (BLAS function)
	// when there is a Go implementation and mat64 has a symmetric matrix.
	yMat := mat64.NewDense(b.dim, 1, b.y)
	yMatTrans := mat64.NewDense(1, b.dim, b.y)
	sMat := mat64.NewDense(b.dim, 1, b.s)
	sMatTrans := mat64.NewDense(1, b.dim, b.s)

	var tmp mat64.Dense
	tmp.Mul(b.invHess, yMat)
	tmp.Mul(&tmp, sMatTrans)
	tmp.Scale(-1/sDotY, &tmp)

	var tmp2 mat64.Dense
	tmp2.Mul(yMatTrans, b.invHess)
	tmp2.Mul(sMat, &tmp2)
	tmp2.Scale(-1/sDotY, &tmp2)

	// Update b hessian
	b.invHess.Add(b.invHess, &tmp)
	b.invHess.Add(b.invHess, &tmp2)

	b.invHess.RankOne(b.invHess, firstTermConst, b.s, b.s)

	// update the bfgs stored data to the new iteration
	copy(b.x, l.X)
	copy(b.grad, l.Gradient)

	// Compute the new search direction
	dirmat := mat64.NewDense(b.dim, 1, direction)
	gradmat := mat64.NewDense(b.dim, 1, l.Gradient)

	dirmat.Mul(b.invHess, gradmat) // new direction stored in place
	floats.Scale(-1, direction)
	return 1
}
