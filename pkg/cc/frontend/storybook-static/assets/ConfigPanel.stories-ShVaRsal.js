import{j as t}from"./jsx-runtime-D_zvdyIk.js";import{C as S}from"./ConfigPanel-Bj3KvomE.js";import{a as o,b as a}from"./fixtures-CTg5x2hB.js";import"./Spinner-CVL8Mf24.js";const{fn:b}=__STORYBOOK_MODULE_TEST__,M={title:"Components/ConfigPanel",component:S,decorators:[C=>t.jsx("div",{className:"w-72 bg-bg p-8",children:t.jsx(C,{})})]},e={args:{config:a,services:o}},n={args:{config:null,services:[]}},r={args:{config:a,services:o,onNavigateConfig:b()}},s={args:{config:{...a,networks:[{name:"mainnet",enabled:!0,portOffset:0},{name:"holesky",enabled:!0,portOffset:100},{name:"sepolia",enabled:!0,portOffset:200}]},services:o}};var c,i,m;e.parameters={...e.parameters,docs:{...(c=e.parameters)==null?void 0:c.docs,source:{originalSource:`{
  args: {
    config: mockConfig,
    services: mockServices
  }
}`,...(m=(i=e.parameters)==null?void 0:i.docs)==null?void 0:m.source}}};var f,g,p;n.parameters={...n.parameters,docs:{...(f=n.parameters)==null?void 0:f.docs,source:{originalSource:`{
  args: {
    config: null,
    services: []
  }
}`,...(p=(g=n.parameters)==null?void 0:g.docs)==null?void 0:p.source}}};var l,u,d;r.parameters={...r.parameters,docs:{...(l=r.parameters)==null?void 0:l.docs,source:{originalSource:`{
  args: {
    config: mockConfig,
    services: mockServices,
    onNavigateConfig: fn()
  }
}`,...(d=(u=r.parameters)==null?void 0:u.docs)==null?void 0:d.source}}};var v,k,O;s.parameters={...s.parameters,docs:{...(v=s.parameters)==null?void 0:v.docs,source:{originalSource:`{
  args: {
    config: {
      ...mockConfig,
      networks: [{
        name: 'mainnet',
        enabled: true,
        portOffset: 0
      }, {
        name: 'holesky',
        enabled: true,
        portOffset: 100
      }, {
        name: 'sepolia',
        enabled: true,
        portOffset: 200
      }]
    },
    services: mockServices
  }
}`,...(O=(k=s.parameters)==null?void 0:k.docs)==null?void 0:O.source}}};const N=["Default","Loading","WithManageButton","MultipleNetworks"];export{e as Default,n as Loading,s as MultipleNetworks,r as WithManageButton,N as __namedExportsOrder,M as default};
