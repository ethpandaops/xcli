import{j as o}from"./jsx-runtime-D_zvdyIk.js";import{H as C}from"./Header-Avgz259g.js";import{f as E,a as O}from"./fixtures-CTg5x2hB.js";const{fn:n}=__STORYBOOK_MODULE_TEST__,R={title:"Components/Header",component:C,decorators:[B=>o.jsx("div",{className:"bg-bg p-8",children:o.jsx(B,{})})],args:{services:O,infrastructure:E,mode:"local",stackStatus:"stopped",onStackAction:n(),notificationsEnabled:!0,onToggleNotifications:n()}},r={},s={args:{stackStatus:"running"}},t={args:{stackStatus:"starting",currentPhase:"Build Services"}},a={args:{stackStatus:"stopping",currentPhase:"Stop Infrastructure"}},e={args:{stackStatus:"running",onNavigateConfig:n()}};var c,i,u;r.parameters={...r.parameters,docs:{...(c=r.parameters)==null?void 0:c.docs,source:{originalSource:"{}",...(u=(i=r.parameters)==null?void 0:i.docs)==null?void 0:u.source}}};var p,g,m;s.parameters={...s.parameters,docs:{...(p=s.parameters)==null?void 0:p.docs,source:{originalSource:`{
  args: {
    stackStatus: 'running'
  }
}`,...(m=(g=s.parameters)==null?void 0:g.docs)==null?void 0:m.source}}};var d,S,f;t.parameters={...t.parameters,docs:{...(d=t.parameters)==null?void 0:d.docs,source:{originalSource:`{
  args: {
    stackStatus: 'starting',
    currentPhase: 'Build Services'
  }
}`,...(f=(S=t.parameters)==null?void 0:S.docs)==null?void 0:f.source}}};var l,k,_;a.parameters={...a.parameters,docs:{...(l=a.parameters)==null?void 0:l.docs,source:{originalSource:`{
  args: {
    stackStatus: 'stopping',
    currentPhase: 'Stop Infrastructure'
  }
}`,...(_=(k=a.parameters)==null?void 0:k.docs)==null?void 0:_.source}}};var h,v,x;e.parameters={...e.parameters,docs:{...(h=e.parameters)==null?void 0:h.docs,source:{originalSource:`{
  args: {
    stackStatus: 'running',
    onNavigateConfig: fn()
  }
}`,...(x=(v=e.parameters)==null?void 0:v.docs)==null?void 0:x.source}}};const T=["Stopped","Running","Starting","Stopping","WithConfigButton"];export{s as Running,t as Starting,r as Stopped,a as Stopping,e as WithConfigButton,T as __namedExportsOrder,R as default};
