import{j as n}from"./jsx-runtime-D_zvdyIk.js";import{G as y}from"./GitStatus-uD1p5Hzl.js";import{e as t}from"./fixtures-CTg5x2hB.js";import"./Spinner-CVL8Mf24.js";const B={title:"Components/GitStatus",component:y,decorators:[x=>n.jsx("div",{className:"bg-bg p-8",children:n.jsx(x,{})})]},r={args:{repos:[t[0]]}},e={args:{repos:t}},o={args:{repos:[t[2]]}},s={args:{repos:[{name:"broken-repo",path:"/tmp/broken",branch:"main",aheadBy:0,behindBy:0,hasUncommitted:!1,uncommittedCount:0,latestTag:"",commitsSinceTag:0,isUpToDate:!1,error:"fatal: not a git repository"}]}},a={args:{repos:[]}};var m,c,p;r.parameters={...r.parameters,docs:{...(m=r.parameters)==null?void 0:m.docs,source:{originalSource:`{
  args: {
    repos: [mockRepos[0]]
  }
}`,...(p=(c=r.parameters)==null?void 0:c.docs)==null?void 0:p.source}}};var i,d,g;e.parameters={...e.parameters,docs:{...(i=e.parameters)==null?void 0:i.docs,source:{originalSource:`{
  args: {
    repos: mockRepos
  }
}`,...(g=(d=e.parameters)==null?void 0:d.docs)==null?void 0:g.source}}};var u,h,l;o.parameters={...o.parameters,docs:{...(u=o.parameters)==null?void 0:u.docs,source:{originalSource:`{
  args: {
    repos: [mockRepos[2]]
  }
}`,...(l=(h=o.parameters)==null?void 0:h.docs)==null?void 0:l.source}}};var f,b,S;s.parameters={...s.parameters,docs:{...(f=s.parameters)==null?void 0:f.docs,source:{originalSource:`{
  args: {
    repos: [{
      name: 'broken-repo',
      path: '/tmp/broken',
      branch: 'main',
      aheadBy: 0,
      behindBy: 0,
      hasUncommitted: false,
      uncommittedCount: 0,
      latestTag: '',
      commitsSinceTag: 0,
      isUpToDate: false,
      error: 'fatal: not a git repository'
    }]
  }
}`,...(S=(b=s.parameters)==null?void 0:b.docs)==null?void 0:S.source}}};var k,T,U;a.parameters={...a.parameters,docs:{...(k=a.parameters)==null?void 0:k.docs,source:{originalSource:`{
  args: {
    repos: []
  }
}`,...(U=(T=a.parameters)==null?void 0:T.docs)==null?void 0:U.source}}};const E=["UpToDate","WithDrift","WithUncommitted","WithError","Loading"];export{a as Loading,r as UpToDate,e as WithDrift,s as WithError,o as WithUncommitted,E as __namedExportsOrder,B as default};
