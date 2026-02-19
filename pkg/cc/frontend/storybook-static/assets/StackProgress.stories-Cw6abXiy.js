import{j as o}from"./jsx-runtime-D_zvdyIk.js";import{S as v,d as e,a as C,B as O}from"./StackProgress-DnhpTGoU.js";const{fn:f}=__STORYBOOK_MODULE_TEST__,q={title:"Components/StackProgress",component:v,decorators:[x=>o.jsx("div",{className:"bg-bg p-8",children:o.jsx(x,{})})]},s={args:{phases:e([{phase:"prerequisites",message:"Done"},{phase:"build_xatu_cbt",message:"Done"},{phase:"infrastructure",message:"Starting ClickHouse..."}],null),error:null,title:"Booting Stack"}},a={args:{phases:e([{phase:"prerequisites",message:""},{phase:"build_xatu_cbt",message:""},{phase:"infrastructure",message:""},{phase:"build_services",message:""},{phase:"network_setup",message:""},{phase:"generate_configs",message:""},{phase:"build_cbt_api",message:""},{phase:"start_services",message:""},{phase:"complete",message:""}],null),error:null,title:"Booting Stack"}},n={args:{phases:e([{phase:"prerequisites",message:"Done"},{phase:"build_xatu_cbt",message:"Building..."}],"Build failed: missing dependency github.com/example/pkg"),error:"Build failed: missing dependency github.com/example/pkg",title:"Booting Stack",onRetry:f()}},r={args:{phases:e([{phase:"stop_services",message:"Done"},{phase:"cleanup_orphans",message:"Cleaning..."}],null,C),error:null,title:"Stopping Stack"}},t={args:{phases:e([{phase:"prerequisites",message:"Checking..."}],null,O),error:null,title:"Booting Stack",onCancel:f()}};var i,p,c;s.parameters={...s.parameters,docs:{...(i=s.parameters)==null?void 0:i.docs,source:{originalSource:`{
  args: {
    phases: derivePhaseStates([{
      phase: 'prerequisites',
      message: 'Done'
    }, {
      phase: 'build_xatu_cbt',
      message: 'Done'
    }, {
      phase: 'infrastructure',
      message: 'Starting ClickHouse...'
    }], null),
    error: null,
    title: 'Booting Stack'
  }
}`,...(c=(p=s.parameters)==null?void 0:p.docs)==null?void 0:c.source}}};var g,l,u;a.parameters={...a.parameters,docs:{...(g=a.parameters)==null?void 0:g.docs,source:{originalSource:`{
  args: {
    phases: derivePhaseStates([{
      phase: 'prerequisites',
      message: ''
    }, {
      phase: 'build_xatu_cbt',
      message: ''
    }, {
      phase: 'infrastructure',
      message: ''
    }, {
      phase: 'build_services',
      message: ''
    }, {
      phase: 'network_setup',
      message: ''
    }, {
      phase: 'generate_configs',
      message: ''
    }, {
      phase: 'build_cbt_api',
      message: ''
    }, {
      phase: 'start_services',
      message: ''
    }, {
      phase: 'complete',
      message: ''
    }], null),
    error: null,
    title: 'Booting Stack'
  }
}`,...(u=(l=a.parameters)==null?void 0:l.docs)==null?void 0:u.source}}};var m,h,d;n.parameters={...n.parameters,docs:{...(m=n.parameters)==null?void 0:m.docs,source:{originalSource:`{
  args: {
    phases: derivePhaseStates([{
      phase: 'prerequisites',
      message: 'Done'
    }, {
      phase: 'build_xatu_cbt',
      message: 'Building...'
    }], 'Build failed: missing dependency github.com/example/pkg'),
    error: 'Build failed: missing dependency github.com/example/pkg',
    title: 'Booting Stack',
    onRetry: fn()
  }
}`,...(d=(h=n.parameters)==null?void 0:h.docs)==null?void 0:d.source}}};var S,_,b;r.parameters={...r.parameters,docs:{...(S=r.parameters)==null?void 0:S.docs,source:{originalSource:`{
  args: {
    phases: derivePhaseStates([{
      phase: 'stop_services',
      message: 'Done'
    }, {
      phase: 'cleanup_orphans',
      message: 'Cleaning...'
    }], null, STOP_PHASES),
    error: null,
    title: 'Stopping Stack'
  }
}`,...(b=(_=r.parameters)==null?void 0:_.docs)==null?void 0:b.source}}};var B,k,P;t.parameters={...t.parameters,docs:{...(B=t.parameters)==null?void 0:B.docs,source:{originalSource:`{
  args: {
    phases: derivePhaseStates([{
      phase: 'prerequisites',
      message: 'Checking...'
    }], null, BOOT_PHASES),
    error: null,
    title: 'Booting Stack',
    onCancel: fn()
  }
}`,...(P=(k=t.parameters)==null?void 0:k.docs)==null?void 0:P.source}}};const T=["BootInProgress","BootComplete","BootError","StopInProgress","WithCancel"];export{a as BootComplete,n as BootError,s as BootInProgress,r as StopInProgress,t as WithCancel,T as __namedExportsOrder,q as default};
