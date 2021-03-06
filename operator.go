package flexsql

type OperatorType uint

const (
	_ = iota
	OpMul
	OpDiv
	OpMod
	OpAdd
	OpSub
	OpIsNull
	OpIsNotNull
	OpIsTrue
	OpIsNotTrue
	OpIsFalse
	OpIsNotFalse
	OpIn
	OpNotIn
	OpBetween
	OpNotBetween
	OpLike
	OpNotLike
	OpILike
	OpNotILike
	OpLt
	OpLte
	OpGt
	OpGte
	OpEq
	OpNotEq
	OpNot
	OpAnd
	OpOr
)

type Associativity uint

const (
	_ = iota
	NonAssociative
	LeftAssociative
	RightAssociative
)

type operator interface {
	Expr
	precedence() uint
	associativity() Associativity
	operatorType() OperatorType
	negatable() bool
	negate() Expr
}

func resolveOperatorPrecedence(op operator, c *Compiler) (uint, error) {
	customPrecedence := op.precedence()
	if customPrecedence != 0 {
		return customPrecedence, nil
	}
	precedence := c.precedence(op.operatorType())
	if precedence != 0 {
		return precedence, nil
	}
	return 0, ErrNoPrecedence
}

func resolveOperatorAssociativity(op operator, c *Compiler) (Associativity, error) {
	customAssociativity := op.associativity()
	if customAssociativity != 0 {
		return customAssociativity, nil
	}
	associativity := c.associativity(op.operatorType())
	if associativity != 0 {
		return associativity, nil
	}
	return 0, ErrNoAssociativity
}

type UnaryOperator struct {
	Type                OperatorType
	Symbol              string
	NegatedType         OperatorType
	NegatedSymbol       string
	Expr                Expr
	CustomPrecedence    uint
	CustomAssociativity Associativity
}

func (u *UnaryOperator) isNot() bool {
	return u.Type == OpNot
}

func (u *UnaryOperator) precedence() uint {
	return u.CustomPrecedence
}

func (u *UnaryOperator) associativity() Associativity {
	return u.CustomAssociativity
}

func (u *UnaryOperator) operatorType() OperatorType {
	return u.Type
}

func (u *UnaryOperator) negatable() bool {
	return u.isNot() || (u.NegatedType != 0 && u.NegatedSymbol != "")
}

func (u *UnaryOperator) negate() Expr {
	if u.isNot() {
		return u.Expr
	}
	return &UnaryOperator{
		Type:                u.NegatedType,
		Symbol:              u.NegatedSymbol,
		NegatedType:         u.Type,
		NegatedSymbol:       u.Symbol,
		Expr:                u.Expr,
		CustomPrecedence:    u.CustomPrecedence,
		CustomAssociativity: u.CustomAssociativity,
	}
}

func (u *UnaryOperator) Transform(c *Compiler) Node {
	if u.isNot() {
		if v, ok := (u.Expr).(operator); ok {
			if v.negatable() {
				return v.negate().Transform(c)
			}
		}
	}
	u.Expr = (u.Expr.Transform(c)).(Expr)
	return u
}

func (u *UnaryOperator) Stringify(c *Compiler) error {
	associativity, err := resolveOperatorAssociativity(u, c)
	if err != nil {
		return err
	}
	if associativity == NonAssociative {
		return ErrNonAssociative
	}
	ourPrecedence, err := resolveOperatorPrecedence(u, c)
	if err != nil {
		return err
	}

	write := func(e Expr, needParen bool) error {
		if associativity == RightAssociative {
			c.WriteVerbatim(u.Symbol + " ")
		}
		if needParen {
			if err := stringifyParen(e, c); err != nil {
				return err
			}
		} else {
			if err := e.Stringify(c); err != nil {
				return err
			}
		}
		if associativity == LeftAssociative {
			c.WriteVerbatim(" " + u.Symbol)
		}
		return nil
	}
	v, ok := (u.Expr).(operator)
	if !ok {
		return write(u.Expr, false)
	}
	theirPrecedence, err := resolveOperatorPrecedence(v, c)
	if err != nil {
		return err
	}
	if theirPrecedence < ourPrecedence {
		return write(u.Expr, true)
	}
	return write(u.Expr, false)
}

type BinaryOperator struct {
	Type                OperatorType
	Symbol              string
	NegatedType         OperatorType
	NegatedSymbol       string
	Left                Expr
	Right               Expr
	CustomPrecedence    uint
	CustomAssociativity Associativity
	SuppressSpace       bool
}

func (b *BinaryOperator) precedence() uint {
	return b.CustomPrecedence
}

func (b *BinaryOperator) associativity() Associativity {
	return b.CustomAssociativity
}

func (b *BinaryOperator) operatorType() OperatorType {
	return b.Type
}

func (b *BinaryOperator) negatable() bool {
	return b.NegatedType != 0 && b.NegatedSymbol != ""
}

func (b *BinaryOperator) negate() Expr {
	return &BinaryOperator{
		Type:                b.NegatedType,
		Symbol:              b.NegatedSymbol,
		NegatedType:         b.Type,
		NegatedSymbol:       b.Symbol,
		Left:                b.Left,
		Right:               b.Right,
		CustomPrecedence:    b.CustomPrecedence,
		CustomAssociativity: b.CustomAssociativity,
	}
}

func (b *BinaryOperator) Transform(c *Compiler) Node {
	b.Left = (b.Left.Transform(c)).(Expr)
	b.Right = (b.Right.Transform(c)).(Expr)
	return b
}

func (b *BinaryOperator) Stringify(c *Compiler) error {
	assoc, err := resolveOperatorAssociativity(b, c)
	if err != nil {
		return err
	}
	ourPrecedence, err := resolveOperatorPrecedence(b, c)
	if err != nil {
		return err
	}

	handleSide := func(e Expr, targetAssoc Associativity) error {
		op, ok := e.(operator)
		if !ok {
			return e.Stringify(c)
		}
		theirPrecedence, err := resolveOperatorPrecedence(op, c)
		if err != nil {
			return err
		}
		needParen := ((assoc == NonAssociative && theirPrecedence <= ourPrecedence) ||
			(assoc != NonAssociative && (theirPrecedence < ourPrecedence ||
				theirPrecedence == ourPrecedence && assoc == targetAssoc)))
		if needParen {
			return stringifyParen(op, c)
		}
		return op.Stringify(c)
	}

	if err := handleSide(b.Left, RightAssociative); err != nil {
		return err
	}
	if b.SuppressSpace {
		c.WriteVerbatim(b.Symbol)
	} else {
		c.WriteVerbatim(" " + b.Symbol + " ")
	}
	return handleSide(b.Right, LeftAssociative)
}

type TernaryOperator struct {
	Type                OperatorType
	Symbol1             string
	Symbol2             string
	NegatedType         OperatorType
	NegatedSymbol1      string
	NegatedSymbol2      string
	Expr1               Expr
	Expr2               Expr
	Expr3               Expr
	CustomPrecedence    uint
	CustomAssociativity Associativity
}

func (t *TernaryOperator) precedence() uint {
	return t.CustomPrecedence
}

func (t *TernaryOperator) associativity() Associativity {
	return t.CustomAssociativity
}

func (t *TernaryOperator) operatorType() OperatorType {
	return t.Type
}

func (t *TernaryOperator) negatable() bool {
	return t.NegatedType != 0 && t.NegatedSymbol1 != "" && t.NegatedSymbol2 != ""
}

func (t *TernaryOperator) negate() Expr {
	return &TernaryOperator{
		Type:                t.NegatedType,
		Symbol1:             t.NegatedSymbol1,
		Symbol2:             t.NegatedSymbol2,
		NegatedType:         t.Type,
		NegatedSymbol1:      t.Symbol1,
		NegatedSymbol2:      t.Symbol2,
		Expr1:               t.Expr1,
		Expr2:               t.Expr2,
		Expr3:               t.Expr3,
		CustomPrecedence:    t.CustomPrecedence,
		CustomAssociativity: t.CustomAssociativity,
	}
}

func (t *TernaryOperator) Transform(c *Compiler) Node {
	t.Expr1 = (t.Expr1.Transform(c)).(Expr)
	t.Expr2 = (t.Expr2.Transform(c)).(Expr)
	t.Expr3 = (t.Expr3.Transform(c)).(Expr)
	return t
}

func (t *TernaryOperator) Stringify(c *Compiler) error {
	ourPrecedence, err := resolveOperatorPrecedence(t, c)
	if err != nil {
		return err
	}

	handleExpr := func(e Expr) error {
		op, ok := e.(operator)
		if !ok {
			return e.Stringify(c)
		}
		theirPrecedence, err := resolveOperatorPrecedence(op, c)
		if err != nil {
			return err
		}
		needParen := theirPrecedence <= ourPrecedence
		if needParen {
			return stringifyParen(op, c)
		}
		return op.Stringify(c)
	}

	if err := handleExpr(t.Expr1); err != nil {
		return err
	}
	c.WriteVerbatim(" " + t.Symbol1 + " ")
	if err := handleExpr(t.Expr2); err != nil {
		return err
	}
	c.WriteVerbatim(" " + t.Symbol2 + " ")
	return handleExpr(t.Expr3)
}

func Not(e Expr) *UnaryOperator {
	return &UnaryOperator{
		Type:   OpNot,
		Symbol: "NOT",
		Expr:   e,
	}
}

func IsNull(e Expr) *UnaryOperator {
	return &UnaryOperator{
		Type:          OpIsNull,
		Symbol:        "IS NULL",
		NegatedType:   OpIsNotNull,
		NegatedSymbol: "IS NOT NULL",
		Expr:          e,
	}
}

func IsNotNull(e Expr) *UnaryOperator {
	return &UnaryOperator{
		Type:          OpIsNotNull,
		Symbol:        "IS NOT NULL",
		NegatedType:   OpIsNull,
		NegatedSymbol: "IS NULL",
		Expr:          e,
	}
}

func IsTrue(e Expr) *UnaryOperator {
	return &UnaryOperator{
		Type:          OpIsTrue,
		Symbol:        "IS TRUE",
		NegatedType:   OpIsNotTrue,
		NegatedSymbol: "IS NOT TRUE",
		Expr:          e,
	}
}

func IsNotTrue(e Expr) *UnaryOperator {
	return &UnaryOperator{
		Type:          OpIsNotTrue,
		Symbol:        "IS NOT TRUE",
		NegatedType:   OpIsTrue,
		NegatedSymbol: "IS TRUE",
		Expr:          e,
	}
}

func IsFalse(e Expr) *UnaryOperator {
	return &UnaryOperator{
		Type:          OpIsFalse,
		Symbol:        "IS FALSE",
		NegatedType:   OpIsNotFalse,
		NegatedSymbol: "IS NOT FALSE",
		Expr:          e,
	}
}

func IsNotFalse(e Expr) *UnaryOperator {
	return &UnaryOperator{
		Type:          OpIsNotFalse,
		Symbol:        "IS NOT FALSE",
		NegatedType:   OpIsFalse,
		NegatedSymbol: "IS FALSE",
		Expr:          e,
	}
}

func And(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:   OpAnd,
		Symbol: "AND",
		Left:   left,
		Right:  right,
	}
}

func Or(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:   OpOr,
		Symbol: "OR",
		Left:   left,
		Right:  right,
	}
}

func Add(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:   OpAdd,
		Symbol: "+",
		Left:   left,
		Right:  right,
	}
}

func Sub(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:   OpSub,
		Symbol: "-",
		Left:   left,
		Right:  right,
	}
}

func Mul(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:   OpMul,
		Symbol: "*",
		Left:   left,
		Right:  right,
	}
}

func Div(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:   OpDiv,
		Symbol: "/",
		Left:   left,
		Right:  right,
	}
}

func Mod(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:   OpMod,
		Symbol: "%",
		Left:   left,
		Right:  right,
	}
}

func Lt(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:   OpLt,
		Symbol: "<",
		Left:   left,
		Right:  right,
	}
}

func Lte(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:   OpLte,
		Symbol: "<=",
		Left:   left,
		Right:  right,
	}
}

func Gt(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:   OpGt,
		Symbol: ">",
		Left:   left,
		Right:  right,
	}
}

func Gte(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:   OpGte,
		Symbol: ">=",
		Left:   left,
		Right:  right,
	}
}

func Eq(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:          OpEq,
		Symbol:        "=",
		NegatedType:   OpNotEq,
		NegatedSymbol: "<>",
		Left:          left,
		Right:         right,
	}
}

func NotEq(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:          OpNotEq,
		Symbol:        "<>",
		NegatedType:   OpEq,
		NegatedSymbol: "=",
		Left:          left,
		Right:         right,
	}
}

func In(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:          OpIn,
		Symbol:        "IN",
		NegatedType:   OpNotIn,
		NegatedSymbol: "NOT IN",
		Left:          left,
		Right:         right,
	}
}

func NotIn(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:          OpNotIn,
		Symbol:        "NOT IN",
		NegatedType:   OpIn,
		NegatedSymbol: "IN",
		Left:          left,
		Right:         right,
	}
}

func Like(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:          OpLike,
		Symbol:        "LIKE",
		NegatedType:   OpNotLike,
		NegatedSymbol: "NOT LIKE",
		Left:          left,
		Right:         right,
	}
}

func NotLike(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:          OpNotLike,
		Symbol:        "NOT LIKE",
		NegatedType:   OpLike,
		NegatedSymbol: "LIKE",
		Left:          left,
		Right:         right,
	}
}

func ILike(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:          OpILike,
		Symbol:        "ILIKE",
		NegatedType:   OpNotILike,
		NegatedSymbol: "NOT ILIKE",
		Left:          left,
		Right:         right,
	}
}

func NotILike(left, right Expr) *BinaryOperator {
	return &BinaryOperator{
		Type:          OpNotILike,
		Symbol:        "NOT ILIKE",
		NegatedType:   OpILike,
		NegatedSymbol: "ILIKE",
		Left:          left,
		Right:         right,
	}
}

func Between(expr1, expr2, expr3 Expr) *TernaryOperator {
	return &TernaryOperator{
		Type:           OpBetween,
		Symbol1:        "BETWEEN",
		Symbol2:        "AND",
		NegatedType:    OpNotBetween,
		NegatedSymbol1: "NOT BETWEEN",
		NegatedSymbol2: "AND",
		Expr1:          expr1,
		Expr2:          expr2,
		Expr3:          expr3,
	}
}

func NotBetween(expr1, expr2, expr3 Expr) *TernaryOperator {
	return &TernaryOperator{
		Type:           OpNotBetween,
		Symbol1:        "NOT BETWEEN",
		Symbol2:        "AND",
		NegatedType:    OpBetween,
		NegatedSymbol1: "BETWEEN",
		NegatedSymbol2: "AND",
		Expr1:          expr1,
		Expr2:          expr2,
		Expr3:          expr3,
	}
}
