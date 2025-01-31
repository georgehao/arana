//
// Licensed to Apache Software Foundation (ASF) under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Apache Software Foundation (ASF) licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.
//

package filter

import (
	"github.com/dubbogo/arana/pkg/proto"
)

var (
	filterFactories = make(map[string]proto.FilterFactory)
	filters         = make(map[string]proto.Filter)
)

func RegistryFilterFactory(name string, factory proto.FilterFactory) {
	filterFactories[name] = factory
}

func GetFilterFactory(name string) proto.FilterFactory {
	return filterFactories[name]
}

func RegisterFilter(name string, filter proto.Filter) {
	filters[name] = filter
}

func GetFilter(name string) proto.Filter {
	return filters[name]
}
