package lifecycle_test

import (
	"encoding/xml"
	"fmt"
	"testing"

	"github.com/hughe/goamz/s3/lifecycle"

	. "gopkg.in/check.v1"
)

// Boilerplate for go-check, mostly so I can use c.Assert() and c.Check()
func Test(t *testing.T) { TestingT(t) }

type unmarshTests struct{}

var _ = Suite(&unmarshTests{})

const lcExample = `<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
    <Rule>
        <ID>Archive and then delete rule</ID>
        <Filter>
           <Prefix>projectdocs/</Prefix>
        </Filter>
        <Status>Enabled</Status>
       <Transition>
           <Days>30</Days>
           <StorageClass>STANDARD_IA</StorageClass>
        </Transition>
        <Transition>
           <Days>365</Days>
           <StorageClass>GLACIER</StorageClass>
        </Transition>
        <Expiration>
           <Days>3650</Days>
        </Expiration>
    </Rule>
</LifecycleConfiguration>`

// <LifecycleConfiguration>
// 	 <Rule>
// 	   <ID>Test Lifecycle</ID>
// 	   <Filter>
// 	     <Prefix>0000/S00</Prefix>
// 	   </Filter>
// 	   <Status>Enabled</Status>
// 	   <Transition>
// 	     <Days>2</Days>
// 	     <StorageClass>GLACIER</StorageClass>
// 	   </Transition>
// 	 </Rule>
// </LifecycleConfiguration>

func (_ *unmarshTests) TestUnmarshal(c *C) {

	x := lifecycle.Configuration{}
	err := xml.Unmarshal([]byte(lcExample), &x)
	c.Assert(err, IsNil)

	//	spew.Dump(x)

	unc := x.IsUnclean()
	c.Assert(unc, Equals, false)

	c.Check(x.Rules, HasLen, 1)
	r := x.Rules[0]
	c.Check(*r.ID, Equals, "Archive and then delete rule")
	c.Check(*r.Filter.Prefix, Equals, "projectdocs/")
	c.Check(r.Status, Equals, "Enabled")
	c.Check(r.Transitions, HasLen, 2)
	c.Check(*r.Transitions[0].Days, Equals, 30)
}

func (_ *unmarshTests) TestMarshal(c *C) {
	id := "Test Lifecycle"
	prefix := "0000/S00"
	days := 2

	lc := lifecycle.Configuration{
		Rules: []lifecycle.Rule{
			{
				ID: &id,
				Filter: &lifecycle.Filter{
					Prefix: &prefix,
				},
				Status: "Enabled",
				Transitions: []lifecycle.Transition{
					{
						StorageClass: "GLACIER",
						Days:         &days,
					},
				},
			},
		},
	}

	data, err := xml.Marshal(&lc)
	c.Assert(err, IsNil)
	fmt.Printf("data = %#v\n", string(data))

}
