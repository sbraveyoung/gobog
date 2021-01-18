package markdown

import (
	"errors"
	"strconv"
	"strings"

	"github.com/SmartBrave/gobog/pkg/stack"
)

type Markdown struct {
	OriginialText []rune
	current       int
	status        int //should is a s
	output        []rune
}

const (
	NewLine    = iota
	NoneStatus //after of any status
	FirstTitleStatus
	SecondTitleStatus
	ThirdTitleStatus
	FourthTitleStatus
	FivthTitleStatus
	SixthTitleStatus
	QuoteStatus
	Italic
	Strong
	ULList
	OLList
	Image
	BlockQuote
	Paragraph
)

var (
	title = map[int]int{1: FirstTitleStatus, 2: SecondTitleStatus, 3: ThirdTitleStatus, 4: FourthTitleStatus, 5: FivthTitleStatus, 6: SixthTitleStatus}
	eltit = map[int]int{FirstTitleStatus: 1, SecondTitleStatus: 2, ThirdTitleStatus: 3, FourthTitleStatus: 4, FivthTitleStatus: 5, SixthTitleStatus: 6}
	s     stack.Stack
)

func (md *Markdown) Parse() (output []rune, err error) {
	//NoneStatus is the first and end status.Any statu should be nest.
	if len(md.OriginialText) == 0 {
		return []rune{}, errors.New("input should not nil.")
	}
	var str []rune
	md.entryStatus(NoneStatus)
	for md.current < len(md.OriginialText) {
		c := int32(md.OriginialText[md.current])
		switch md.status {
		case NoneStatus:
			switch c {
			case '\n':
				//if walk(1),could not check err
				md.walk(1)
			case '*':
				StarNum := md.readContinuousChar('*')
				switch StarNum {
				case 1:
					if str, err = md.getNextnRunes(2); err != nil {
						return md.output, nil
					}
					if md.isNewLine() && strings.HasPrefix(string(str), "* ") { //列表
						md.output = append(md.output, []rune("<ul><li>")...)
						md.entryStatus(ULList)
						if err = md.walk(2); err != nil {
							return []rune{}, err
						}
					} else { //斜体
						md.output = append(md.output, []rune("<p><em>")...)
						md.entryStatus(Italic)
						md.walk(1)
					}
				case 2: //粗体
					md.output = append(md.output, []rune("<p><strong>")...)
					md.entryStatus(Strong)
					if err = md.walk(2); err != nil {
						return []rune{}, err
					}
				case 3:
					str := string(md.readLine())
					line := strings.TrimSpace(str)
					if md.isNewLine() && strings.Compare(line, "***") == 0 {
						md.output = append(md.output, []rune("<hr>")...)
						if err = md.walk(len(str)); err != nil {
							return []rune{}, err
						}
					} else {
						md.output = append(md.output, []rune("<p>")...)
						md.entryStatus(Paragraph)
					}
				default:
					md.output = append(md.output, []rune("<p>")...)
					md.entryStatus(Paragraph)
				}
			case '>':
				if md.isNewLine() {
					md.output = append(md.output, []rune("<blockquote>")...)
					md.entryStatus(BlockQuote)
					str, err = md.getNextnRunes(2)
					if err == nil {
						if str[1] == ' ' {
							md.walk(1)
						}
					}
				} else {
					md.output = append(md.output, c)
				}
				md.walk(1)
			//case '`':
			case '!':
				if str, err = md.getNextnRunes(2); err != nil {
					return md.output, err
				}
				if strings.Compare(string(str), "![") == 0 {
					md.output = append(md.output, []rune("<p><img alt=\"")...)
					md.entryStatus(Image)
					if err = md.walk(2); err != nil {
						return md.output, err
					}
				}
			case '[':
				str = md.readLine()
				var description, href []rune
				var long int
				if description, href, long, err = md.unmarshalA(string(str)); err != nil {
					//return md.output,err
					md.output = append(md.output, c)
					md.walk(1)
				}
				md.output = append(md.output, []rune("<p><a href=\""+string(href)+"\">"+string(description)+"</a></p>")...)
				if err = md.walk(long); err != nil {
					return []rune{}, err
				}
			case '#':
				SharpNum := md.readContinuousChar('#')
				str, err := md.getNextnRunes(SharpNum + 1)
				if err != nil || str[SharpNum] != ' ' { //not title
					md.output = append(md.output, []rune("<p>")...)
					md.entryStatus(Paragraph)
					//should not walk
					continue
				}
				//title
				if err = md.walk(int(SharpNum) + 1); err != nil {
					return []rune{}, err
				}
				if SharpNum > 6 {
					SharpNum = 6
				}
				md.output = append(md.output, []rune("<h"+strconv.Itoa(SharpNum)+">")...)
				md.entryStatus(title[SharpNum])
			default: //'#'
				md.entryStatus(Paragraph)
				md.output = append(md.output, []rune("<p>")...)
				md.output = append(md.output, c)
				md.walk(1)
			}
		case Paragraph:
			md.handleParagraph(c)
		case Image:
			md.handleImage(c)
		case Italic:
			md.handleEMText(c)
		case Strong:
			md.handleStrongText(c)
		case ULList:
			md.handleULList(c)
		case FirstTitleStatus, SecondTitleStatus, ThirdTitleStatus, FourthTitleStatus, FivthTitleStatus, SixthTitleStatus:
			md.handleTitle(c)
		case BlockQuote:
			md.handleBlockQuote(c)
		default:
		}
	}
	return md.output, nil
}

func (md *Markdown) unmarshalA(str string) (desc, href []rune, long int, err error) {
	//[description](url)
	var status string
	long = 1
	for cur := 0; cur < len(str); cur++ {
		long++
		if str[cur] == '[' {
			status = "d"
			continue
		} else if str[cur] == ']' && md.in(1) && str[cur+1] == '(' {
			status = "h"
			cur++
			continue
		} else if str[cur] == ')' {
			return
		}

		if strings.Compare(status, "d") == 0 {
			desc = append(desc, rune(str[cur]))
		}
		if strings.Compare(status, "h") == 0 {
			href = append(href, rune(str[cur]))
		}
	}
	return []rune{}, []rune{}, 0, errors.New("unknown error.")
}

func (md *Markdown) readLine() (str []rune) {
	//BUG:last line has no '\n',return error
	cur := md.current
	for cur < len(md.OriginialText) {
		if md.OriginialText[cur] == '\n' {
			return
		}
		str = append(str, md.OriginialText[cur])
		cur++
	}
	return
}

func (md *Markdown) isTitle() bool {
	if md.status == FirstTitleStatus || md.status == SecondTitleStatus ||
		md.status == ThirdTitleStatus || md.status == FourthTitleStatus ||
		md.status == FivthTitleStatus || md.status == SixthTitleStatus {
		return true
	}
	return false
}

func (md *Markdown) entryStatus(statu int) {
	//md.status is the top of stack forover
	s.Push(statu)
	md.status = statu
}

func (md *Markdown) outStatus(statu int) (nextStatus int) {
	if !s.IsEmpty() {
		if s.Pop().(int) == statu {
			return s.Top().(int)
		}
	}
	return -1
}

func (md *Markdown) walk(n int) error {
	if md.current+n <= len(md.OriginialText) {
		md.current += n
		return nil
	}
	return errors.New("the index out of range.")
}

func (md *Markdown) back(n int) error {
	if md.current-n <= len(md.OriginialText) {
		md.current -= n
		return nil
	}
	return errors.New("the index out of range.")
}

func (md *Markdown) readContinuousChar(ch rune) int { //should is rune,not byte,because rune is int32,byte is uint8
	i := 1
	var str []rune
	var err error
	for md.in(i) {
		str, err = md.getNextnRunes(i)
		if err != nil || str[i-1] != ch {
			return i - 1
		}
		i++
	}
	return i - 1
}

func (md *Markdown) getNextnRunes(n int) (str []rune, err error) {
	if md.in(n) {
		if n >= 0 {
			return md.OriginialText[md.current : md.current+n], nil
		} else {
			return md.OriginialText[md.current+n+1 : md.current+1], nil
		}
	}
	return []rune{}, errors.New("the index out of range.")
}

func (md *Markdown) in(n int) bool { //next n bytes of current position
	if n >= 0 {
		return md.current >= 0 && md.current+n <= len(md.OriginialText)
	} else {
		return md.current+n >= 0 && md.current <= len(md.OriginialText)
	}
}

func (md *Markdown) handleStar(c int32) (err error) {
	return
}

func (md *Markdown) handleTitle(c int32) (err error) {
	switch c {
	case '\n': //should not drop \n
		md.output = append(md.output, []rune("</h"+strconv.Itoa(eltit[md.status])+">")...)
		md.walk(1)
		md.status = md.outStatus(md.status)
	case ' ':
		md.output = append(md.output, []rune("&nbsp;")...)
		md.walk(1)
	default:
		md.output = append(md.output, c)
		if !md.in(2) {
			md.output = append(md.output, []rune("</h"+strconv.Itoa(eltit[md.status])+">")...)
		}
		md.walk(1)
	}
	return
}

func (md *Markdown) handleParagraph(c int32) (err error) {
	var str []rune
	switch c {
	case '\n':
		LRNum := md.readContinuousChar('\n')
		if LRNum >= 2 {
			md.status = md.outStatus(Paragraph)
			md.output = append(md.output, []rune("</p>")...)
			md.back(1)
		}
		if !md.in(2) {
			md.output = append(md.output, []rune("</p>")...)
			md.status = md.outStatus(Paragraph)
			//I have no other idea
			if md.status == BlockQuote {
				md.output = append(md.output, []rune("</blockquote>")...)
			}
		}
		if err = md.walk(LRNum); err != nil {
			return err
		}
	case '*':
		StarNum := md.readContinuousChar('*')
		switch StarNum {
		case 1:
			if str, err = md.getNextnRunes(2); err != nil {
				return nil
			}
			if md.isNewLine() && strings.HasPrefix(string(str), "* ") { //列表
				md.output = append(md.output, []rune("<ul><li>")...)
				md.entryStatus(ULList)
				if err = md.walk(2); err != nil {
					return err
				}
			} else { //斜体
				md.output = append(md.output, []rune("<p><em>")...)
				md.entryStatus(Italic)
				md.walk(1)
			}
		case 2: //粗体
			md.output = append(md.output, []rune("<p><strong>")...)
			md.entryStatus(Strong)
			if err = md.walk(2); err != nil {
				return err
			}
		case 3:
			str := string(md.readLine())
			line := strings.TrimSpace(str)
			if md.isNewLine() && strings.Compare(line, "***") == 0 {
				md.output = append(md.output, []rune("<hr>")...)
				if err = md.walk(len(str)); err != nil {
					return err
				}
			} else {
				md.output = append(md.output, []rune("<p>")...)
				md.entryStatus(Paragraph)
			}
		default:
			md.output = append(md.output, []rune("<p>")...)
			md.entryStatus(Paragraph)
		}
	case '`':
	case '!':
	case '[':
	default:
		md.output = append(md.output, c)
		if !md.in(2) {
			md.output = append(md.output, []rune("</p>")...)
			md.status = md.outStatus(Paragraph)
			//I have no other idea too
			if md.status == BlockQuote {
				md.output = append(md.output, []rune("</blockquote>")...)
			}
		}
		md.walk(1)
	}
	return
}

func (md *Markdown) handleULList(c int32) (err error) {
	str := []rune{}
	switch c {
	case '\n':
		if str, err = md.getNextnRunes(3); err != nil {
			md.output = append(md.output, []rune("</li></ul>")...)
			md.walk(1)
			md.status = md.outStatus(ULList)
			return
		}
		if strings.Compare(string(str), "\n* ") == 0 {
			md.output = append(md.output, []rune("</li><li>")...)
			if err = md.walk(3); err != nil {
				return err
			}
		} else {
			md.output = append(md.output, []rune("</li></ul>")...)
			md.walk(1)
			md.status = md.outStatus(ULList)
			return
		}
	case ' ':
		md.output = append(md.output, []rune("&nbsp;")...)
		md.walk(1)
	default:
		md.output = append(md.output, c)
		md.walk(1)
	}
	return
}

func (md *Markdown) handleOLList(current int32) (err error) {
	return
}

func (md *Markdown) handleStrongText(c int32) (err error) {
	switch c {
	case '*':
		StarNum := md.readContinuousChar('*')
		switch StarNum {
		case 2:
			md.output = append(md.output, []rune("</strong></p>")...)
			md.status = md.outStatus(Strong)
		default:
		}
		if err = md.walk(int(StarNum)); err != nil {
			return err
		}
	case ' ':
		md.output = append(md.output, []rune("&nbsp;")...)
		md.walk(1)
	default:
		md.output = append(md.output, c)
		md.walk(1)
	}
	return
}

func (md *Markdown) handleEMText(c int32) (err error) {
	switch c {
	case '*':
		StarNum := md.readContinuousChar('*')
		switch StarNum {
		case 1:
			md.output = append(md.output, []rune("</em></p>")...)
			md.status = md.outStatus(md.status)
		default:
		}
		if err = md.walk(int(StarNum)); err != nil {
			return err
		}
	case ' ':
		md.output = append(md.output, []rune("&nbsp;")...)
		md.walk(1)
	default:
		md.output = append(md.output, c)
		md.walk(1)
	}
	return
}

func (md *Markdown) handleHref(current int32) (err error) {
	return
}

func (md *Markdown) handleImage(c int32) (err error) {
	switch c {
	case ']':
		md.output = append(md.output, []rune("\"")...)
	case '(':
		md.output = append(md.output, []rune(" src=\"")...)
	case ')':
		md.output = append(md.output, []rune("\"></p>")...)
		md.status = md.outStatus(Image)
	case '\n':
		md.output = append(md.output, []rune("></p>")...)
		md.status = md.outStatus(Image)
	default:
		md.output = append(md.output, c)
	}
	md.walk(1)
	return
}

func (md *Markdown) handleCode(current int32) (err error) {
	return
}

func (md *Markdown) handleBlockQuote(c int32) (err error) {
	var str []rune
	switch c {
	case ' ':
		md.output = append(md.output, []rune("&nbsp;")...)
		md.walk(1)
	case '\n':
		str, err := md.getNextnRunes(2)
		if err != nil || str[1] == '\n' {
			md.output = append(md.output, []rune("</blockquote>")...)
			md.status = md.outStatus(BlockQuote)
			md.walk(1)
			return nil
		}
		md.walk(1)
	case '>':
		if !md.isNewLine() {
			md.output = append(md.output, c)
		}
		md.walk(1)
	default:
		str, err = md.getNextnRunes(-3)
		if err == nil && str[0] == '\n' && str[1] == '\n' {
			md.output = append(md.output, []rune("</blockquote>")...)
			md.status = md.outStatus(BlockQuote)
			return
		}
		md.output = append(md.output, []rune("<p>")...)
		md.entryStatus(Paragraph)
	}
	return
}

func (md *Markdown) isNewLine() bool {
	str := []rune{}
	var err error
	if md.in(-2) {
		if str, err = md.getNextnRunes(-2); err != nil {
			return false
		}
		if str[0] != '\n' {
			return false
		}
	}
	return true
}
