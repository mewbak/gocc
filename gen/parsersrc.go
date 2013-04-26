//Copyright 2012 Vastech SA (PTY) LTD
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package gen

const parserSrcHdr = `
package parser

import(
	"fmt"
	"strconv"
	"errors"
)

`

const parserSrcBody = `
type(
	ActionTab []*ActionRow
	ActionRow struct {
		CanRecover bool
		Actions Actions
	}
	Actions map[token.Type]Action
)

func (R *ActionRow) String() string {
	s := fmt.Sprintf("CanRecover=%t\n", R.CanRecover) 
	for t, a := range R.Actions {
		s += strconv.Itoa(int(t)) + " : " + a.String() + "\n"
	}
	return s
}

type(
	Accept int
	Shift State
	Reduce int

	Action interface{
		Act()
		String() string
	}

)

func (this Accept) Act(){}
func (this Shift) Act(){}
func (this Reduce) Act(){}

func (this Accept) String() string { return "Accept(0)" }
func (this Shift) String() string { return "Shift(" + strconv.Itoa(int(this)) + ")" }
func (this Reduce) String() string { return "Reduce(" + strconv.Itoa(int(this)) + ")" }

type(
	GotoTab []GotoRow
	GotoRow	map[NT] State
	State int
	NT	string
)

type (
	ProdTab []ProdTabEntry
	ProdTabEntry struct {
		String		string
		Head		NT
		NumSymbols	int
		ReduceFunc	func([]Attrib) (Attrib, error)
	}
	Attrib interface{
//		String() string
	}
)

// Stack

type stack struct {
	state []State
	attrib	[]Attrib
}

const INITIAL_STACK_SIZE = 100

func NewStack() *stack {
	return &stack{ 	state: 	make([]State, 0, INITIAL_STACK_SIZE),
					attrib: make([]Attrib, 0, INITIAL_STACK_SIZE),
			}
}

func (this *stack) reset() {
	this.state = this.state[0:0]
	this.attrib = this.attrib[0:0]
}

func (this *stack) Push(s State, a Attrib) {
	this.state = append(this.state, s)
	this.attrib = append(this.attrib, a)
}

func(this *stack) Top() State {
	return this.state[len(this.state) - 1]
}

func (this *stack) Peek(pos int) State {
	return this.state[pos]
}

func (this *stack) TopIndex() int {
	return len(this.state) - 1
}

func (this *stack) PopN(items int) []Attrib {
	lo, hi := len(this.state) - items, len(this.state)
	
	attrib := this.attrib[lo: hi]
	
	this.state = this.state[:lo]
	this.attrib = this.attrib[:lo]
	
	return attrib
}

func (S *stack) String() string {
	res := "stack:\n"
	for i, st := range S.state {
		res += "\t" + strconv.Itoa(i) + ": " + strconv.Itoa(int(st))
		res += " , "
		if S.attrib[i] == nil {
			res += "nil"
		} else {
			res += fmt.Sprintf("%v", S.attrib[i])
		}
		res += "\n"
	}
	return res
}

// Parser

type Parser struct {
	actTab ActionTab
	gotoTab	GotoTab
	prodTab	ProdTab
	stack	*stack
	nextToken	*token.Token
	pos	token.Position
	tokenMap *token.TokenMap
}

type Scanner interface {
	Scan() (*token.Token, token.Position)
}

func NewParser(act ActionTab, gto GotoTab, prod ProdTab, tm *token.TokenMap) *Parser {
	p := &Parser{actTab: act, gotoTab: gto, prodTab: prod, stack:NewStack(), tokenMap:tm}
	p.stack.Push(0, nil) //TODO: which attribute should be pushed here?
	return p
}

func (P *Parser) Reset() {
	P.stack.reset()
	P.stack.Push(0, nil) //TODO: which attribute should be pushed here?
}

func Acc() {
	fmt.Println("Accept")
}

func (P *Parser) Error(err error, scanner Scanner) (recovered bool, errorAttrib *errs.Error) {
	errorAttrib = &errs.Error{
		Err: err,
		ErrorToken: P.nextToken,
		ErrorPos: P.pos,
		ErrorSymbols: P.popNonRecoveryStates(),
		ExpectedTokens: make([]string, 0, 8),
	}
	for t, _ := range P.actTab[P.stack.Top()].Actions {
		errorAttrib.ExpectedTokens = append(errorAttrib.ExpectedTokens, P.tokenMap.TokenString(t))
	}


	action, ok := P.actTab[P.stack.Top()].Actions[P.tokenMap.Type("error")]
	if !ok {
		return
	}
	P.stack.Push(State(action.(Shift)), errorAttrib) // action can only be shift

	_, recovered = P.actTab[P.stack.Top()].Actions[P.nextToken.Type]
	for !recovered && P.nextToken.Type != token.EOF {
		P.nextToken, P.pos = scanner.Scan()
		_, recovered = P.actTab[P.stack.Top()].Actions[P.nextToken.Type]
	}

	return
}

func (P *Parser) popNonRecoveryStates() (removedAttribs []errs.ErrorSymbol) {
	if rs, ok := P.firstRecoveryState(); ok {
		errorSymbols := P.stack.PopN(int(P.stack.TopIndex() - rs))
		removedAttribs = make([]errs.ErrorSymbol, len(errorSymbols))
		for i, e := range errorSymbols {
			removedAttribs[i] = e
		}
	} else {
		removedAttribs = []errs.ErrorSymbol{}
	}
	return
}

// recoveryState points to the highest state on the stack, which can recover
func (P *Parser) firstRecoveryState() (recoveryState int, canRecover bool) {
	recoveryState, canRecover = P.stack.TopIndex(), P.actTab[P.stack.Top()].CanRecover
	for recoveryState > 0 && !canRecover {
		recoveryState--
		canRecover = P.actTab[P.stack.Peek(recoveryState)].CanRecover
	}
	return
}


func (P *Parser) newError(err error) error {
	errmsg := "Error: " + P.TokString(P.nextToken) + " @ " + P.pos.String()
	if err != nil {
		errmsg += " " + err.Error()
	} else {
		errmsg += ", expected one of: "
		actRow := P.actTab[P.stack.Top()]
		i := 0
		for t := range actRow.Actions {
			errmsg += P.tokenMap.TokenString(t)
			if i < len(actRow.Actions)-1 {
				errmsg += " "
			}
			i++
		}
	}
	return errors.New(errmsg)
}

func (P *Parser) TokString(tok *token.Token) string {
	msg := P.tokenMap.TokenString(tok.Type) + "(" + strconv.Itoa(int(tok.Type)) + ")"
	msg += " " + string(tok.Lit)
	return msg
}

func (this *Parser) Parse(scanner Scanner) (res interface{}, err error) {
	this.Reset()
	this.nextToken, this.pos = scanner.Scan()
	for acc := false; !acc; {
		action, ok := this.actTab[this.stack.Top()].Actions[this.nextToken.Type]
		if !ok {
			if recovered, errAttrib := this.Error(nil, scanner); !recovered {
				this.nextToken, this.pos = errAttrib.ErrorToken, errAttrib.ErrorPos
				return nil, this.newError(nil)
			}
			if action, ok = this.actTab[this.stack.Top()].Actions[this.nextToken.Type]; !ok {
				panic("Error recover led to invalid action")
			}
		}
		switch act := action.(type) {
		case Accept:
			res = this.stack.PopN(1)[0]
			acc = true
		case Shift:
			this.stack.Push(State(act), this.nextToken)
			this.nextToken, this.pos = scanner.Scan()
		case Reduce:
			prod := this.prodTab[int(act)]
			attrib, err := prod.ReduceFunc(this.stack.PopN(prod.NumSymbols))
			if err != nil {
				return nil, this.newError(err)
			} else {
				this.stack.Push(this.gotoTab[this.stack.Top()][prod.Head], attrib)
			}
		default:
			panic("unknown action: " + action.String())
		}
	}
	return res, nil
}
`
