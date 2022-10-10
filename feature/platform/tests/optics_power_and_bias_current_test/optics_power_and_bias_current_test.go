// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package optics_power_and_bias_current_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openconfig/featureprofiles/internal/components"
	"github.com/openconfig/featureprofiles/internal/fptest"
	"github.com/openconfig/ondatra"
	"github.com/openconfig/ondatra/telemetry"
	"github.com/openconfig/ygot/ygot"
)

const (
	transceiverType        = telemetry.PlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_TRANSCEIVER
	sleepDuration          = time.Minute
	minOpticsPower         = -30.0
	maxOpticsPower         = 10.0
	minOpticsHighThreshold = 1.0
	maxOpticsLowThreshold  = -1.0
)

func TestMain(m *testing.M) {
	fptest.RunTests(m)
}

// Topology:
//   ate:port1 <--> port1:dut:port2 <--> ate:port2
//
//  Sample CLI command to get telemetry using gmic:
//   - gnmic -a ipaddr:10162 -u username -p password --skip-verify get \
//      --path /components/component --format flat
//   - gnmic tool info:
//     - https://github.com/karimra/gnmic/blob/main/README.md
//

func TestOpticsPowerBiasCurrent(t *testing.T) {
	dut := ondatra.DUT(t, "dut")

	transceivers := components.FindComponentsByType(t, dut, transceiverType)
	t.Logf("Found transceiver list: %v", transceivers)
	if len(transceivers) == 0 {
		t.Fatalf("Get transceiver list for %q: got 0, want > 0", dut.Model())
	}

	for _, transceiver := range transceivers {
		t.Logf("Validate transceiver: %s", transceiver)
		component := dut.Telemetry().Component(transceiver)

		if !component.MfgName().Lookup(t).IsPresent() {
			t.Logf("component.MfgName().Lookup(t).IsPresent() for %q is false. skip it", transceiver)
			continue
		}
		mfgName := component.MfgName().Get(t)
		t.Logf("Transceiver %s MfgName: %s", transceiver, mfgName)

		inputPowers := component.Transceiver().ChannelAny().InputPower().Instant().Get(t)
		t.Logf("Transceiver %s inputPowers: %v", transceiver, inputPowers)
		if len(inputPowers) == 0 {
			t.Errorf("Get inputPowers list for %q: got 0, want > 0", transceiver)
		}
		outputPowers := component.Transceiver().ChannelAny().OutputPower().Instant().Get(t)
		t.Logf("Transceiver %s outputPowers: %v", transceiver, outputPowers)
		if len(outputPowers) == 0 {
			t.Errorf("Get outputPowers list for %q: got 0, want > 0", transceiver)
		}

		biasCurrents := component.Transceiver().ChannelAny().LaserBiasCurrent().Instant().Get(t)
		t.Logf("Transceiver %s biasCurrents: %v", transceiver, biasCurrents)
		if len(outputPowers) == 0 {
			t.Errorf("Get biasCurrents list for %q: got 0, want > 0", transceiver)
		}
	}
}

func TestOpticsPowerUpdate(t *testing.T) {
	dut := ondatra.DUT(t, "dut")
	dp := dut.Port(t, "port1")
	d := &telemetry.Device{}
	i := d.GetOrCreateInterface(dp.Name())

	cases := []struct {
		desc                string
		IntfStatus          bool
		expectedStatus      telemetry.E_Interface_OperStatus
		expectedMaxOutPower float64
		checkMinOutPower    bool
	}{{
		// Check both input and output optics power are in normal range.
		desc:                "Check initial input and output optics powers are OK",
		IntfStatus:          true,
		expectedStatus:      telemetry.Interface_OperStatus_UP,
		expectedMaxOutPower: maxOpticsPower,
		checkMinOutPower:    true,
	}, {
		desc:                "Check output optics power is very small after interface is disabled",
		IntfStatus:          false,
		expectedStatus:      telemetry.Interface_OperStatus_DOWN,
		expectedMaxOutPower: minOpticsPower,
		checkMinOutPower:    false,
	}, {
		desc:                "Check output optics power is normal after interface is re-enabled",
		IntfStatus:          true,
		expectedStatus:      telemetry.Interface_OperStatus_UP,
		expectedMaxOutPower: maxOpticsPower,
		checkMinOutPower:    true,
	}}
	for _, tc := range cases {
		t.Log(tc.desc)
		intUpdateTime := 2 * time.Minute
		t.Run(tc.desc, func(t *testing.T) {
			i.Enabled = ygot.Bool(tc.IntfStatus)
			dut.Config().Interface(dp.Name()).Replace(t, i)
			dut.Telemetry().Interface(dp.Name()).OperStatus().Await(t, intUpdateTime, tc.expectedStatus)

			transceiverName, err := findTransceiverName(dut, dp.Name())
			if err != nil {
				t.Fatalf("findTransceiver(%s, %s): %v", dut.Name(), dp.Name(), err)
			}

			component := dut.Telemetry().Component(transceiverName)
			if !component.MfgName().Lookup(t).IsPresent() {
				t.Skipf("component.MfgName().Lookup(t).IsPresent() for %q is false. skip it", transceiverName)
			}

			mfgName := component.MfgName().Get(t)
			t.Logf("Transceiver MfgName: %s", mfgName)

			channels := dut.Telemetry().Component(dp.Name()).Transceiver().ChannelAny()
			inputPowers := channels.InputPower().Instant().Get(t)
			outputPowers := channels.OutputPower().Instant().Get(t)
			for _, inPower := range inputPowers {
				if inPower > maxOpticsPower || inPower < minOpticsPower {
					t.Errorf("Get inputPower for port %q): got %.2f, want within [%f, %f]", dp.Name(), inPower, minOpticsPower, maxOpticsPower)
				}
			}
			for _, outPower := range outputPowers {
				if outPower > tc.expectedMaxOutPower {
					t.Errorf("Get outPower for port %q): got %.2f, want < %f", dp.Name(), outPower, tc.expectedMaxOutPower)
				}
				if tc.checkMinOutPower && outPower < minOpticsPower {
					t.Errorf("Get outPower for port %q): got %.2f, want > %f", dp.Name(), outPower, minOpticsPower)
				}
			}
		})
	}
}

// findTransceiverName provides name of transciever port corresponding to interface name
func findTransceiverName(dut *ondatra.DUTDevice, interfaceName string) (string, error) {
	var (
		transceiverMap = map[ondatra.Vendor]string{
			ondatra.ARISTA:  " transceiver",
			ondatra.CISCO:   "",
			ondatra.JUNIPER: "",
		}
	)
	transceiverName := interfaceName
	name, ok := transceiverMap[dut.Vendor()]
	if !ok {
		return "", fmt.Errorf("No transceiver interface available for DUT vendor %v", dut.Vendor())
	}
	if name != "" {
		interfaceSplit := strings.Split(interfaceName, "/")
		interfaceSplitres := interfaceSplit[:len(interfaceSplit)-1]
		transceiverName = strings.Join(interfaceSplitres, "/") + name

	}
	return transceiverName, nil
}